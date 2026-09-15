package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"driftbottle/internal/ai"
	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
)

func (s *Service) Activity(ctx context.Context, u string) (any, string, error) {
	var raw []byte
	var expires time.Time
	err := s.Store.DB.QueryRowContext(ctx, db.ActivitySelect, u).Scan(&raw, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "unavailable", nil
	}
	if err != nil {
		return nil, "unavailable", err
	}
	var p ai.Profile
	if err = json.Unmarshal(raw, &p); err != nil {
		return nil, "unavailable", err
	}
	status := "fresh"
	if expires.Before(time.Now()) {
		status = "stale"
	}
	return p, status, nil
}
func (s *Service) Draft(ctx context.Context, u, id, kind string, version int) (any, error) {
	b, err := s.ownedBottle(ctx, s.Store.DB, u, id, false)
	if err != nil {
		return nil, err
	}
	if b.Version != version {
		return nil, domain.Conflict("STALE_AI_DRAFT")
	}
	activity, status, err := s.Activity(ctx, u)
	if err != nil {
		return nil, err
	}
	input := map[string]any{"question": b.Raw, "hint": b.Hint, "activity": activity}
	if kind == "target" && s.Zhihu.Secret != "" {
		if search, e := s.Search(ctx, u, b.Raw); e == nil {
			input["publicContext"] = search.Items
		}
	}
	var out ai.Draft
	if err = s.AI.Run(ctx, kind, input, &out); err != nil {
		return nil, err
	}
	v := map[string]any{"bottleId": domain.Public("btl", id), "sourceContentVersion": version, "needsClarification": out.Clarify, "question": out.Question}
	if kind == "episode" {
		if out.Title == "" || out.Summary == "" {
			return nil, domain.Fail(503, "AI_PROTOCOL_ERROR", "内容处理结果不完整")
		}
		v["title"] = out.Title
		v["summary"] = out.Summary
	} else {
		if len(out.Required) == 0 && !out.Clarify {
			return nil, domain.Fail(503, "AI_PROTOCOL_ERROR", "目标建议不完整")
		}
		t := domain.Target{Required: out.Required, Preferred: out.Preferred, Viewpoints: out.Viewpoints}
		t.Normalize()
		v["requiredExperiences"] = t.Required
		v["preferredExperiences"] = t.Preferred
		v["viewpointPreferences"] = t.Viewpoints
		v["activityUsed"] = activity != nil
		v["activityStatus"] = status
	}
	return v, nil
}
func (s *Service) Suggestion(ctx context.Context, u, id string) (any, error) {
	var status string
	var raw []byte
	err := s.Store.DB.QueryRowContext(ctx, db.SuggestionSelect, id, u).Scan(&status, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.NotFound
	}
	return map[string]any{"status": status, "suggestions": json.RawMessage(raw)}, err
}
