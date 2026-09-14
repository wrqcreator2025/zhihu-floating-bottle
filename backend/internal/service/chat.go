package service

import (
	"context"
	"database/sql"
	"errors"

	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
)

func (s *Service) InviteChat(ctx context.Context, u, id string) (any, error) {
	var inv, status string
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		c, err := s.connection(ctx, tx, u, id, true)
		if err != nil {
			return err
		}
		if c.Seeker != u {
			return domain.NotFound
		}
		if c.Status == "closed" {
			return domain.Conflict("CONNECTION_CLOSED")
		}
		blocked, err := db.Blocked(ctx, tx, u, c.Responder)
		if err != nil {
			return err
		}
		if blocked {
			return domain.Conflict("USER_BLOCKED")
		}
		if c.Status != "replied" {
			return domain.Conflict("REPLY_REQUIRED")
		}
		err = tx.QueryRowContext(ctx, db.InviteChatSelect, id).Scan(&inv, &status)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		inv = domain.ID()
		status = "pending"
		_, err = tx.ExecContext(ctx, db.InviteChatInsert, inv, id)
		if err != nil {
			return err
		}
		return db.Notify(ctx, tx, c.Responder, "chat_invited", "connection", id, "chat-invite:"+inv, "对方邀请你继续聊聊")
	})
	return map[string]any{"id": domain.Public("chi", inv), "status": status}, err
}
func (s *Service) ChatStatus(ctx context.Context, u, id string) (any, error) {
	if _, err := s.connection(ctx, s.Store.DB, u, id, false); err != nil {
		return nil, err
	}
	var inv, status string
	var session, sessionStatus sql.NullString
	err := s.Store.DB.QueryRowContext(ctx, db.ChatStatusSelect, id).Scan(&inv, &status, &session, &sessionStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{"invitation": nil, "session": nil}, nil
	}
	if err != nil {
		return nil, err
	}
	var sv any
	if session.Valid {
		sv = map[string]any{"id": domain.Public("cht", session.String), "status": sessionStatus.String}
	}
	return map[string]any{"invitation": map[string]any{"id": domain.Public("chi", inv), "status": status}, "session": sv}, nil
}
func (s *Service) DecideChat(ctx context.Context, u, id, decision string) (any, error) {
	var cid, status, sid string
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, db.DecideChatSelect, id).Scan(&cid)
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
		if c.Responder != u {
			return domain.NotFound
		}
		if err = tx.QueryRowContext(ctx, db.DecideChatSelect2, id).Scan(&status); err != nil {
			return err
		}
		wanted := "accepted"
		if decision == "decline" {
			wanted = "declined"
		}
		if status != "pending" {
			if status != wanted {
				return domain.Conflict("INVITATION_ALREADY_DECIDED")
			}
			return nil
		}
		if c.Status == "closed" {
			return domain.Conflict("CONNECTION_CLOSED")
		}
		blocked, err := db.Blocked(ctx, tx, u, c.Seeker)
		if err != nil {
			return err
		}
		if blocked {
			return domain.Conflict("USER_BLOCKED")
		}
		_, err = tx.ExecContext(ctx, db.DecideChatUpdate, wanted, id)
		if err != nil {
			return err
		}
		status = wanted
		if status == "accepted" {
			sid = domain.ID()
			_, err = tx.ExecContext(ctx, db.DecideChatInsert, sid, cid, id)
			if err != nil {
				return err
			}
		}
		return db.Notify(ctx, tx, c.Seeker, "chat_decided", "connection", cid, "chat-decision:"+id, "聊天邀请已有回应")
	})
	if err != nil {
		return nil, err
	}
	return s.ChatStatus(ctx, u, cid)
}
func (s *Service) Report(ctx context.Context, u, id, reason, messageID string) (any, error) {
	c, err := s.connection(ctx, s.Store.DB, u, id, false)
	if err != nil {
		return nil, err
	}
	var mid any
	if messageID != "" {
		var visible bool
		err = s.Store.DB.QueryRowContext(ctx, db.ReportSelect, messageID, c.ID).Scan(&visible)
		if err != nil {
			return nil, err
		}
		if !visible {
			return nil, domain.NotFound
		}
		mid = messageID
	}
	rid := domain.ID()
	_, err = s.Store.DB.ExecContext(ctx, db.ReportInsert, rid, u, id, mid, reason)
	return map[string]any{"id": domain.Public("rpt", rid), "status": "pending"}, err
}
func (s *Service) Block(ctx context.Context, u, id string) error {
	return s.Store.Tx(ctx, func(tx *sql.Tx) error {
		c, err := s.connection(ctx, tx, u, id, true)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, db.BlockInsert, u, c.Other(u))
		return err
	})
}
