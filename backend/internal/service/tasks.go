package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"driftbottle/internal/ai"
	"driftbottle/internal/domain"
	"driftbottle/internal/matching"
	"driftbottle/internal/moderation"
	db "driftbottle/internal/repository/mysql"
)

func (s *Service) review(ctx context.Context, kind, id string, version int, input any, force bool) (ai.Review, error) {
	var result ai.Review
	var status, reason string
	var appeal sql.NullString
	err := s.Store.DB.QueryRowContext(ctx, db.ReviewSelect, kind, id, version).Scan(&status, &reason, &appeal)
	if err == nil && !force && (status == "approved" || status == "rejected") {
		allowed := status == "approved"
		return ai.Review{Allowed: &allowed, Reason: reason}, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}

	if err = s.AI.Run(ctx, "moderation", map[string]any{"kind": kind, "content": input, "appeal": appeal.String}, &result); err != nil {
		return result, err
	}
	if result.Allowed == nil {
		return result, domain.Fail(503, "AI_PROTOCOL_ERROR", "审核结果缺少明确结论")
	}
	status = "rejected"
	if *result.Allowed && result.SuggestedRoute == "" {
		status = "approved"
	} else {
		v := false
		result.Allowed = &v
	}
	if result.Reason == "" {
		result.Reason = "content_not_allowed"
	}
	if len(result.Reason) > 64 {
		result.Reason = "content_not_allowed"
	}
	_, err = s.Store.DB.ExecContext(ctx, db.ReviewInsert, domain.ID(), kind, id, version, hash(domain.JSON(input)), status, result.Reason)
	return result, err
}
func (s *Service) MatchBottle(ctx context.Context, p domain.JobPayload) error {
	b, err := db.Bottle(ctx, s.Store.DB, p.ID, false)
	if err != nil {
		return err
	}
	if b.Status != "searching" || b.Round != p.Round {
		return nil
	}
	// Bottle text is input to recommendations, not a moderation gate.
	// Messages exchanged between users retain their separate moderation flow.
	var count int
	if err = s.Store.DB.QueryRowContext(ctx, db.SearchStatusSelect, b.ID, b.Round).Scan(&count); err != nil {
		return err
	}
	activity, _, err := s.Activity(ctx, b.Owner)
	if err != nil {
		slog.WarnContext(ctx, "activity unavailable for matching", "bottle_id", b.ID)
		activity = nil
	}
	target := s.matchTarget(ctx, b, activity)
	candidates := map[string]domain.Experience{}
	ranked := []ai.Match{}
	cursor := ""
	for count < s.AttemptLimit {
		rows, e := s.Store.DB.QueryContext(ctx, db.MatchBottleSelect, b.Owner, cursor, b.ID, b.Owner, b.Owner)
		if e != nil {
			return e
		}
		inputs := []any{}
		for rows.Next() {
			var exp domain.Experience
			var disclosure, profile []byte
			if e = rows.Scan(&exp.ID, &exp.Owner, &exp.Title, &exp.Body, &disclosure, &profile); e != nil {
				rows.Close()
				return e
			}
			if e = json.Unmarshal(disclosure, &exp.Disclosure); e != nil {
				rows.Close()
				return e
			}
			cursor = exp.ID
			candidates[exp.ID] = exp
			inputs = append(inputs, map[string]any{"experienceId": exp.ID, "title": exp.Title, "body": exp.Body, "activity": json.RawMessage(profile)})
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(inputs) > 0 {
			var out ai.Matches
			if e = s.AI.Run(ctx, "match", map[string]any{"question": b.Raw, "target": target, "activity": activity, "candidates": inputs}, &out); e != nil {
				slog.WarnContext(ctx, "AI match unavailable; using broad candidate order", "bottle_id", b.ID)
				out.Items = broadMatches(inputs)
			}
			seen := map[string]bool{}
			valid := true
			for _, item := range out.Items {
				if _, ok := candidates[item.ExperienceID]; !ok || seen[item.ExperienceID] {
					valid = false
					break
				}
				seen[item.ExperienceID] = true
			}
			if !valid {
				slog.WarnContext(ctx, "AI match returned invalid candidates; using broad candidate order", "bottle_id", b.ID)
				out.Items = broadMatches(inputs)
				seen = map[string]bool{}
				for _, item := range out.Items {
					seen[item.ExperienceID] = true
				}
			}
			if len(out.Items) < len(inputs) {
				slog.WarnContext(ctx, "AI match returned partial candidates", "expected", len(inputs), "actual", len(out.Items))
				for _, input := range inputs {
					id, ok := input.(map[string]any)["experienceId"].(string)
					if ok && !seen[id] {
						out.Items = append(out.Items, ai.Match{ExperienceID: id, Eligible: true, Score: 0.1})
					}
				}
			}
			ranked = append(ranked, matching.Order(out.Items)...)
		}
		users := map[string]bool{}
		for _, m := range ranked {
			users[candidates[m.ExperienceID].Owner] = true
		}
		if len(users) >= s.AttemptLimit-count || len(inputs) < 200 {
			break
		}
	}
	return s.Store.Tx(ctx, func(tx *sql.Tx) error {
		current, e := db.Bottle(ctx, tx, b.ID, true)
		if e != nil {
			return e
		}
		if current.Status != "searching" || current.Round != b.Round || current.Version != b.Version {
			return nil
		}
		if e = tx.QueryRowContext(ctx, db.SearchStatusSelect, b.ID, b.Round).Scan(&count); e != nil {
			return e
		}
		for _, m := range ranked {
			if count >= s.AttemptLimit {
				break
			}
			exp, ok := candidates[m.ExperienceID]
			if !ok {
				continue
			}
			var title, body string
			var disclosure []byte
			e = tx.QueryRowContext(ctx, db.MatchBottleSelect2, exp.ID, exp.Owner).Scan(&title, &body, &disclosure)
			if errors.Is(e, sql.ErrNoRows) {
				continue
			}
			if e != nil {
				return e
			}
			if title != exp.Title || body != exp.Body {
				continue
			}
			if e = json.Unmarshal(disclosure, &exp.Disclosure); e != nil {
				return e
			}
			blocked, e := db.Blocked(ctx, tx, b.Owner, exp.Owner)
			if e != nil {
				return e
			}
			if blocked {
				continue
			}
			iid := domain.ID()
			_, e = tx.ExecContext(ctx, db.MatchBottleInsert, iid, b.ID, b.Round, exp.Owner, exp.ID, domain.JSON(exp.Snapshot()), "对方想听有相似亲身经历的人说说")
			if db.Duplicate(e) {
				continue
			}
			if e != nil {
				return e
			}
			count++
			if e = db.Notify(ctx, tx, exp.Owner, "invitation_received", "invitation", iid, "invitation:"+iid, "海上漂来一个瓶子"); e != nil {
				return e
			}
		}
		var pending, accepted int
		e = tx.QueryRowContext(ctx, db.MatchBottleSelect3, b.ID, b.Round).Scan(&pending, &accepted)
		if e != nil || pending > 0 {
			return e
		}
		status, reason := "match_failed", "all_declined_or_expired"
		if count == 0 {
			reason = "no_candidates"
		}
		if accepted > 0 {
			status = "completed"
			reason = ""
		}
		_, e = tx.ExecContext(ctx, db.MatchBottleUpdate2, status, reason, b.ID)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, db.BottleActionDelete, b.ID)
		if e != nil {
			return e
		}
		if status == "match_failed" {
			return db.Notify(ctx, tx, b.Owner, "match_failed", "bottle", b.ID, fmt.Sprintf("match-failed:%s:%d", b.ID, b.Round), "这次暂时没有找到合适的人")
		}
		return nil
	})
}

func (s *Service) matchTarget(ctx context.Context, b domain.Bottle, activity any) domain.Target {
	var out ai.Draft
	err := s.AI.Run(ctx, "target", map[string]any{"question": b.Raw, "hint": b.Hint, "activity": activity, "confirmedTarget": b.Target}, &out)
	if err == nil && (len(out.Required) > 0 || len(out.Preferred) > 0 || len(out.Viewpoints) > 0) {
		target := domain.Target{Required: out.Required, Preferred: out.Preferred, Viewpoints: out.Viewpoints}
		target.Normalize()
		if len(target.Required) > 0 || len(target.Preferred) > 0 || len(target.Viewpoints) > 0 {
			return target
		}
	}
	if err != nil {
		slog.WarnContext(ctx, "AI target unavailable; using submitted target", "bottle_id", b.ID)
	}
	target := b.Target
	target.Normalize()
	if len(target.Required) == 0 {
		if strings.TrimSpace(b.Hint) != "" {
			target.Required = []string{strings.TrimSpace(b.Hint)}
		} else {
			target.Required = []string{"亲身经历过与这段描述相似的处境"}
		}
	}
	return target
}

func broadMatches(inputs []any) []ai.Match {
	items := make([]ai.Match, 0, len(inputs))
	for _, input := range inputs {
		id, ok := input.(map[string]any)["experienceId"].(string)
		if ok {
			items = append(items, ai.Match{ExperienceID: id, Eligible: true, Score: 0.1})
		}
	}
	return items
}

func (s *Service) ModerateMessage(ctx context.Context, p domain.JobPayload) error {
	var cid, body, status string
	var version int
	err := s.Store.DB.QueryRowContext(ctx, db.ModerateMessageSelect, p.ID).Scan(&cid, &body, &status, &version)
	if err != nil {
		return err
	}
	if version != p.Version || status != "pending_moderation" {
		return nil
	}
	local := moderation.Check(body)
	var review ai.Review
	if local.Action == moderation.NeedsAI {
		review, err = s.review(ctx, "message", p.ID, version, body, false)
		if err != nil {
			return err
		}
	} else {
		allowed := local.Action == moderation.Allow
		review = ai.Review{Allowed: &allowed, Reason: local.Reason}
		status := "rejected"
		if allowed {
			status = "approved"
		}
		_, err = s.Store.DB.ExecContext(ctx, db.ReviewInsert, domain.ID(), "message", p.ID, version, hash(domain.JSON(body)), status, local.Reason)
		if err != nil {
			return err
		}
	}
	return s.Store.Tx(ctx, func(tx *sql.Tx) error {
		c, e := db.Connection(ctx, tx, cid, true)
		if e != nil {
			return e
		}
		var sender, kind, current string
		var v int
		e = tx.QueryRowContext(ctx, db.ModerateMessageSelect2, p.ID).Scan(&sender, &kind, &current, &v)
		if e != nil {
			return e
		}
		if v != version || current != "pending_moderation" {
			return nil
		}
		blocked, e := db.Blocked(ctx, tx, c.Seeker, c.Responder)
		if e != nil {
			return e
		}
		status := "rejected"
		if *review.Allowed && !blocked && c.Status != "closed" {
			status = "delivered"
		}
		_, e = tx.ExecContext(ctx, db.ModerateMessageUpdate, status, status, p.ID)
		if e != nil {
			return e
		}
		if status != "delivered" {
			return nil
		}
		notification := "chat_message_received"
		if kind == "reply" {
			_, e = tx.ExecContext(ctx, db.ModerateMessageUpdate2, cid)
			notification = "first_reply_received"
		} else {
			_, e = tx.ExecContext(ctx, db.ModerateMessageUpdate3, cid)
		}
		if e != nil {
			return e
		}
		return db.Notify(ctx, tx, c.Other(sender), notification, "connection", cid, "message:"+p.ID, "有人寄来了一封信")
	})
}
func (s *Service) GenerateSlice(ctx context.Context, p domain.JobPayload) error {
	var cid, author, status string
	err := s.Store.DB.QueryRowContext(ctx, db.GenerateSliceSelect, p.ID).Scan(&cid, &author, &status)
	if err != nil || status != "generating" {
		return err
	}
	rows, err := s.Store.DB.QueryContext(ctx, db.GenerateSliceSelect2, cid, author)
	if err != nil {
		return err
	}
	texts := []string{}
	for rows.Next() {
		var t string
		if err = rows.Scan(&t); err != nil {
			rows.Close()
			return err
		}
		texts = append(texts, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(texts) == 0 {
		return domain.Conflict("REPLY_REQUIRED")
	}
	var out ai.Slice
	if err = s.AI.Run(ctx, "slice", texts, &out); err != nil {
		return err
	}
	if out.Title == "" || out.Body == "" {
		return domain.Fail(503, "AI_PROTOCOL_ERROR", "草稿结果不完整")
	}
	_, err = s.Store.DB.ExecContext(ctx, db.GenerateSliceUpdate, out.Title, out.Body, p.ID)
	return err
}
