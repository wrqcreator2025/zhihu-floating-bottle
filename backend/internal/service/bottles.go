package service

import (
	"context"
	"database/sql"
	"strings"
	"unicode/utf8"

	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
)

type BottleCreate struct {
	EpisodeText string `json:"episodeText"`
	TargetHint  string `json:"targetHint"`
}
type EpisodePatch struct {
	Raw       *string `json:"rawText"`
	Title     *string `json:"title"`
	Confirmed *bool   `json:"confirmed"`
}
type TargetPatch struct {
	Hint       *string   `json:"hintText"`
	Required   *[]string `json:"requiredExperiences"`
	Preferred  *[]string `json:"preferredExperiences"`
	Viewpoints *[]string `json:"viewpointPreferences"`
}

func (t TargetPatch) Apply(b *domain.Bottle) {
	if t.Hint != nil {
		b.Hint = *t.Hint
	}
	if t.Required != nil {
		b.Target.Required = *t.Required
	}
	if t.Preferred != nil {
		b.Target.Preferred = *t.Preferred
	}
	if t.Viewpoints != nil {
		b.Target.Viewpoints = *t.Viewpoints
	}
}

type BottlePatch struct {
	Version *int          `json:"sourceContentVersion"`
	Episode *EpisodePatch `json:"episode"`
	Target  *TargetPatch  `json:"target"`
}

func (s *Service) CreateBottle(ctx context.Context, u string, in BottleCreate) (any, error) {
	id := domain.ID()
	_, err := s.Store.DB.ExecContext(ctx, db.CreateBottleInsert, id, u, in.EpisodeText, in.TargetHint)
	if err != nil {
		return nil, err
	}
	b, err := db.Bottle(ctx, s.Store.DB, id, false)
	return b.View(), err
}
func (s *Service) EditBottle(ctx context.Context, u, id string, in BottlePatch) (any, error) {
	var b domain.Bottle
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		b, err = s.ownedBottle(ctx, tx, u, id, true)
		if err != nil {
			return err
		}
		if b.Status != "draft" {
			return domain.Conflict("INVALID_BOTTLE_STATE")
		}
		if in.Version != nil && *in.Version != b.Version {
			return domain.Conflict("STALE_AI_DRAFT")
		}
		if in.Episode != nil {
			p := in.Episode
			if p.Raw != nil {
				b.Raw = *p.Raw
				b.Confirmed = false
			}
			if p.Title != nil {
				b.Title = *p.Title
			}
			if p.Confirmed != nil {
				b.Confirmed = *p.Confirmed
			}
		}
		if in.Target != nil {
			in.Target.Apply(&b)
		}
		b.Version++
		var confirmed any
		if b.Confirmed {
			confirmed = b.Raw
		}
		_, err = tx.ExecContext(ctx, db.EditBottleUpdate, b.Raw, b.Title, confirmed, b.Hint, domain.JSON(b.Target), b.Version, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	b, err = db.Bottle(ctx, s.Store.DB, id, false)
	return b.View(), err
}
func (s *Service) BottleAction(ctx context.Context, u, id, action string, target *TargetPatch) (any, error) {
	var b domain.Bottle
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		b, err = s.ownedBottle(ctx, tx, u, id, true)
		if err != nil {
			return err
		}
		if action == "pause" {
			if b.Status == "paused" {
				return nil
			}
			if b.Status != "searching" {
				return domain.Conflict("INVALID_BOTTLE_STATE")
			}
			_, err = tx.ExecContext(ctx, db.BottleActionUpdate, id)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, db.BottleActionDelete, id)
			b.Status = "paused"
			return err
		}
		if b.Status == "searching" {
			return nil
		}
		allowed := action == "launch" && b.Status == "draft" || action == "resume" && b.Status == "paused" || action == "retry" && (b.Status == "match_failed" || b.Status == "search_error")
		if !allowed {
			return domain.Conflict("INVALID_BOTTLE_STATE")
		}
		if target != nil {
			target.Apply(&b)
			b.Version++
		}
		if !b.Confirmed || len(b.Target.Required) == 0 {
			return domain.Conflict("TARGET_CONFIRMATION_REQUIRED")
		}
		// Lock the owner once to serialize capacity decisions for different bottles.
		var lockedUser string
		if err = tx.QueryRowContext(ctx, db.BottleActionLockUser, u).Scan(&lockedUser); err != nil {
			return err
		}
		var activeCount int
		if err = tx.QueryRowContext(ctx, db.BottleActionCount, u).Scan(&activeCount); err != nil {
			return err
		}
		if activeCount >= s.ActiveBottleLimit {
			rows, e := tx.QueryContext(ctx, db.BottleActionSelect, u)
			if e != nil {
				return e
			}
			activeIDs := []string{}
			for rows.Next() {
				var activeID string
				if e = rows.Scan(&activeID); e != nil {
					rows.Close()
					return e
				}
				activeIDs = append(activeIDs, domain.Public("btl", activeID))
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			return &domain.Error{Status: 409, Code: "ACTIVE_BOTTLE_LIMIT_REACHED", Message: "同时寻找的瓶子已达到上限", Details: map[string]any{"limit": s.ActiveBottleLimit, "activeBottleIds": activeIDs}}
		}
		_, err = tx.ExecContext(ctx, db.BottleActionInsert, u, id)
		if db.Duplicate(err) {
			return domain.Conflict("BOTTLE_ALREADY_SEARCHING")
		}
		if err != nil {
			return err
		}
		if action != "resume" {
			b.Round++
		}
		_, err = tx.ExecContext(ctx, db.BottleActionUpdate2, b.Round, b.Version, domain.JSON(b.Target), b.Hint, id)
		if err != nil {
			return err
		}
		b.Status = "searching"
		if action == "launch" {
			if _, err = tx.ExecContext(ctx, db.BottleExperienceUpsert, id, u, bottleExperienceTitle(b), b.Raw); err != nil {
				return err
			}
		}
		return db.Enqueue(ctx, tx, "match_bottle", domain.JobPayload{ID: id, Round: b.Round}, "")
	})
	if err != nil {
		return nil, err
	}
	if action == "launch" {
		return map[string]any{"bottleId": domain.Public("btl", id), "status": b.Status, "interaction": "throw_to_sea"}, nil
	}
	b, err = db.Bottle(ctx, s.Store.DB, id, false)
	return b.View(), err
}
func (s *Service) BottleDetail(ctx context.Context, u, id string) (any, error) {
	b, err := s.ownedBottle(ctx, s.Store.DB, u, id, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.Store.DB.QueryContext(ctx, db.BottleDetailSelect, u, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, status string
		var updated any
		var unread bool
		if err = rows.Scan(&id, &status, &updated, &unread); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": domain.Public("con", id), "status": status, "updatedAt": updated, "unread": unread})
	}
	return map[string]any{"bottle": b.View(), "connections": out}, rows.Err()
}
func (s *Service) SearchStatus(ctx context.Context, u, id string) (any, error) {
	b, err := s.ownedBottle(ctx, s.Store.DB, u, id, false)
	if err != nil {
		return nil, err
	}
	var n int
	err = s.Store.DB.QueryRowContext(ctx, db.SearchStatusSelect, id, b.Round).Scan(&n)
	v := map[string]any{"status": b.Status, "attemptedCount": n, "attemptLimit": s.AttemptLimit, "updatedAt": b.Updated, "message": "还在寻找同时符合经历且愿意接住的人"}
	if b.Failure != nil {
		v["reason"] = *b.Failure
	}
	if b.Status == "match_failed" {
		v["availableActions"] = []string{"retry", "edit_and_retry"}
	}
	if b.Status == "search_error" {
		v["availableActions"] = []string{"retry"}
	}
	return v, err
}

// Appeal retains the original version and asks for a fresh review when re-launched.
func (s *Service) AppealBottle(ctx context.Context, u, id, reason string) (any, error) {
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		b, err := s.ownedBottle(ctx, tx, u, id, true)
		if err != nil {
			return err
		}
		if b.Status != "draft" || b.Failure == nil || *b.Failure != "content_rejected" {
			return domain.Conflict("INVALID_BOTTLE_STATE")
		}
		_, err = tx.ExecContext(ctx, db.AppealBottleUpdate, reason, id, b.Version)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.BottleAction(ctx, u, id, "launch", nil)
}

func bottleExperienceTitle(b domain.Bottle) string {
	title := strings.TrimSpace(b.Title)
	if title == "" {
		title = strings.TrimSpace(b.Raw)
	}
	if utf8.RuneCountInString(title) > 40 {
		runes := []rune(title)
		title = string(runes[:40])
	}
	if title == "" {
		return "一段正在经历的事"
	}
	return title
}
