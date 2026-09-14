package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
)

func (s *Service) ConnectionDetail(ctx context.Context, u, id string) (any, error) {
	c, err := s.connection(ctx, s.Store.DB, u, id, false)
	if err != nil {
		return nil, err
	}
	b, err := db.Bottle(ctx, s.Store.DB, c.BottleID, false)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = s.Store.DB.QueryRowContext(ctx, db.ConnectionDetailSelect, c.InvitationID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var exp any
	if err = json.Unmarshal(raw, &exp); err != nil {
		return nil, err
	}
	messages, _, err := s.Messages(ctx, u, id, "", 50)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": domain.Public("con", id), "bottleId": domain.Public("btl", c.BottleID), "invitationId": domain.Public("inv", c.InvitationID), "status": c.Status, "createdAt": c.Created, "closedAt": c.Closed, "letter": map[string]any{"episodeTitle": b.Title, "body": b.Raw}, "experience": exp, "messages": messages}, nil
}
func (s *Service) Messages(ctx context.Context, u, id, cursor string, limit int) ([]domain.Message, *string, error) {
	c, err := s.connection(ctx, s.Store.DB, u, id, false)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.Store.DB.QueryContext(ctx, db.MessagesSelect, id, u, cursor, cursor, limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []domain.Message{}
	for rows.Next() {
		var m domain.Message
		var sender string
		if err = rows.Scan(&m.ID, &m.Body, &m.Kind, &m.Status, &m.Version, &sender, &m.Created); err != nil {
			return nil, nil, err
		}
		m.Role = "responder"
		if sender == c.Seeker {
			m.Role = "sender"
		}
		m.ID = domain.Public("msg", m.ID)
		out = append(out, m)
	}
	var next *string
	if len(out) > limit {
		v := out[limit-1].ID
		next = &v
		out = out[:limit]
	}
	return out, next, rows.Err()
}
func (s *Service) SendMessage(ctx context.Context, u, id, body, key, kind string) (any, error) {
	var mid, status string
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		c, err := s.connection(ctx, tx, u, id, true)
		if err != nil {
			return err
		}
		if key != "" {
			var oldBody, oldKind string
			err = tx.QueryRowContext(ctx, db.SendMessageSelect, id, u, key).Scan(&mid, &oldBody, &oldKind, &status)
			if err == nil {
				if oldBody != body || oldKind != kind {
					return domain.Conflict("IDEMPOTENCY_CONFLICT")
				}
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if c.Status == "closed" {
			return domain.Conflict("CONNECTION_CLOSED")
		}
		blocked, err := db.Blocked(ctx, tx, u, c.Other(u))
		if err != nil {
			return err
		}
		if blocked {
			return domain.Conflict("USER_BLOCKED")
		}
		if kind == "reply" {
			if u != c.Responder || c.Status != "awaiting_first_reply" {
				return domain.Conflict("CHAT_INVITATION_REQUIRED")
			}
			var exists bool
			if err = tx.QueryRowContext(ctx, db.SendMessageSelect2, id).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return domain.Conflict("REPLY_ALREADY_SUBMITTED")
			}
		} else {
			var active bool
			err = tx.QueryRowContext(ctx, db.SendMessageSelect3, id).Scan(&active)
			if err != nil {
				return err
			}
			if !active {
				return domain.Conflict("CHAT_NOT_ACCEPTED")
			}
		}
		var seq int
		if err = tx.QueryRowContext(ctx, db.SendMessageSelect4, id).Scan(&seq); err != nil {
			return err
		}
		mid = domain.ID()
		var idem any
		if key != "" {
			idem = key
		}
		_, err = tx.ExecContext(ctx, db.SendMessageInsert, mid, id, u, seq, kind, body, idem)
		if err != nil {
			return err
		}
		status = "pending_moderation"
		return db.Enqueue(ctx, tx, "moderate_message", domain.JobPayload{ID: mid, Version: 1}, "moderate:"+mid+":1")
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"message": map[string]any{"id": domain.Public("msg", mid), "body": body, "deliveryStatus": status}, "interaction": "await_moderation"}, nil
}
func (s *Service) EditMessage(ctx context.Context, u, id string, body *string, appeal string) (any, error) {
	var version int
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		var cid string
		err := tx.QueryRowContext(ctx, db.EditMessageSelect, id, u).Scan(&cid)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.NotFound
		}
		if err != nil {
			return err
		}
		c, err := s.connection(ctx, tx, u, cid, true)
		if err != nil {
			return err
		}
		if c.Status == "closed" {
			return domain.Conflict("CONNECTION_CLOSED")
		}
		var status string
		err = tx.QueryRowContext(ctx, db.EditMessageSelect2, id).Scan(&status, &version)
		if err != nil {
			return err
		}
		if status != "rejected" && status != "moderation_failed" {
			return domain.Conflict("INVALID_MESSAGE_STATE")
		}
		if body != nil {
			version++
			_, err = tx.ExecContext(ctx, db.EditMessageUpdate, *body, version, id)
		} else {
			_, err = tx.ExecContext(ctx, db.EditMessageUpdate2, id)
			if err == nil {
				_, err = tx.ExecContext(ctx, db.EditMessageUpdate3, appeal, id, version)
			}
		}
		if err != nil {
			return err
		}
		return db.Enqueue(ctx, tx, "moderate_message", domain.JobPayload{ID: id, Version: version}, "")
	})
	return map[string]any{"id": domain.Public("msg", id), "contentVersion": version, "deliveryStatus": "pending_moderation"}, err
}
func (s *Service) CloseConnection(ctx context.Context, u, id string) (any, error) {
	var closed *time.Time
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		c, err := s.connection(ctx, tx, u, id, true)
		if err != nil {
			return err
		}
		if c.Status != "closed" {
			_, err = tx.ExecContext(ctx, db.CloseConnectionUpdate, id)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, db.CloseConnectionUpdate2, id)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, db.CloseConnectionUpdate3, id)
			if err != nil {
				return err
			}
		}
		return tx.QueryRowContext(ctx, db.CloseConnectionSelect, id).Scan(&closed)
	})
	return map[string]any{"connectionId": domain.Public("con", id), "status": "closed", "closedAt": closed}, err
}
func (s *Service) Feedback(ctx context.Context, u, id, result string) (any, error) {
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		c, err := s.connection(ctx, tx, u, id, true)
		if err != nil {
			return err
		}
		if c.Seeker != u {
			return domain.NotFound
		}
		var delivered bool
		if err = tx.QueryRowContext(ctx, db.FeedbackSelect, id).Scan(&delivered); err != nil {
			return err
		}
		if !delivered {
			return domain.Conflict("REPLY_REQUIRED")
		}
		_, err = tx.ExecContext(ctx, db.FeedbackInsert, id, u, result)
		if db.Duplicate(err) {
			var existing string
			if err = tx.QueryRowContext(ctx, db.FeedbackSelect2, id).Scan(&existing); err != nil {
				return err
			}
			if existing != result {
				return domain.Conflict("FEEDBACK_ALREADY_SUBMITTED")
			}
			return nil
		}
		return err
	})
	return map[string]any{"connectionId": domain.Public("con", id), "result": result}, err
}
