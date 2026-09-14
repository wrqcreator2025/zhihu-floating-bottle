package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
)

func (s *Service) NextInvitation(ctx context.Context, u string) (any, error) {
	var id, bottle, exp, reason, title, body, hint string
	var expires time.Time
	err := s.Store.DB.QueryRowContext(ctx, db.NextInvitationSelect, u).Scan(&id, &bottle, &exp, &reason, &expires, &title, &body, &hint)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": domain.Public("inv", id), "bottleId": domain.Public("btl", bottle), "matchedExperienceId": domain.Public("exp", exp), "reason": reason, "letter": map[string]any{"episodeTitle": title, "body": body, "wantedExperience": hint}, "status": "pending", "expiresAt": expires}, nil
}
func (s *Service) Decide(ctx context.Context, u, id, decision string) (any, error) {
	var result any
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		var bid string
		if err := tx.QueryRowContext(ctx, db.DecideSelect, id, u).Scan(&bid); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.NotFound
			}
			return err
		}
		b, err := db.Bottle(ctx, tx, bid, true)
		if err != nil {
			return err
		}
		blocked, err := db.Blocked(ctx, tx, u, b.Owner)
		if err != nil {
			return err
		}
		if blocked {
			return domain.Conflict("USER_BLOCKED")
		}
		status := "accepted"
		if decision != "accept" {
			status = "declined_" + decision
		}
		r, err := tx.ExecContext(ctx, db.DecideUpdate, status, id, u)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			var existing string
			var expired bool
			err = tx.QueryRowContext(ctx, db.DecideSelect2, id).Scan(&existing, &expired)
			if err != nil {
				return err
			}
			if existing != status {
				if existing == "expired" || existing == "pending" && expired {
					return domain.Conflict("INVITATION_EXPIRED")
				}
				return domain.Conflict("INVITATION_ALREADY_DECIDED")
			}
		}
		if decision == "accept" {
			var cid string
			if n > 0 {
				cid = domain.ID()
				_, err = tx.ExecContext(ctx, db.DecideInsert, cid, bid, id, b.Owner, u)
				if err != nil {
					return err
				}
				err = db.Notify(ctx, tx, b.Owner, "bottle_accepted", "connection", cid, "accept:"+id, "有人接住了你的瓶子")
				if err != nil {
					return err
				}
			} else {
				err = tx.QueryRowContext(ctx, db.DecideSelect3, id).Scan(&cid)
				if err != nil {
					return err
				}
			}
			result = map[string]any{"invitationId": domain.Public("inv", id), "status": status, "connectionId": domain.Public("con", cid), "interaction": "open_reply_editor"}
		} else {
			result = map[string]any{"invitationId": domain.Public("inv", id), "status": status, "interaction": "return_to_sea", "bottleContinuesMatching": b.Status == "searching"}
		}
		if n > 0 && b.Status == "searching" {
			return db.Enqueue(ctx, tx, "match_bottle", domain.JobPayload{ID: bid, Round: b.Round}, "decision:"+id)
		}
		return nil
	})
	return result, err
}
