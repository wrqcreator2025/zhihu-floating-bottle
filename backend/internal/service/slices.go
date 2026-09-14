package service

import (
	"context"
	"database/sql"
	"errors"

	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
)

func (s *Service) CreateSlice(ctx context.Context, u, cid string) (any, error) {
	var id, status string
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		c, err := s.connection(ctx, tx, u, cid, true)
		if err != nil {
			return err
		}
		if u != c.Responder {
			return domain.NotFound
		}
		if c.Status != "closed" {
			return domain.Conflict("CONNECTION_NOT_CLOSED")
		}
		err = tx.QueryRowContext(ctx, db.CreateSliceSelect, cid).Scan(&id, &status)
		if err == nil {
			if status != "failed" {
				return nil
			}
			_, err = tx.ExecContext(ctx, db.CreateSliceUpdate, id)
		} else if errors.Is(err, sql.ErrNoRows) {
			id = domain.ID()
			_, err = tx.ExecContext(ctx, db.CreateSliceInsert, id, cid, u)
		}
		if err != nil {
			return err
		}
		status = "generating"
		return db.Enqueue(ctx, tx, "generate_slice", domain.JobPayload{ID: id}, "")
	})
	return map[string]any{"id": domain.Public("slc", id), "status": status}, err
}
func (s *Service) Slice(ctx context.Context, u, id string) (any, error) {
	var cid, title, body, status string
	err := s.Store.DB.QueryRowContext(ctx, db.SliceSelect, id, u).Scan(&cid, &title, &body, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.NotFound
	}
	return map[string]any{"id": domain.Public("slc", id), "connectionId": domain.Public("con", cid), "title": title, "body": body, "status": status}, err
}
func (s *Service) EditSlice(ctx context.Context, u, id string, title, body *string, publish bool) (any, error) {
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		var status string
		err := tx.QueryRowContext(ctx, db.EditSliceSelect, id, u).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.NotFound
		}
		if err != nil {
			return err
		}
		if publish && status == "published" {
			return nil
		}
		if status != "ready" {
			return domain.Conflict("INVALID_SLICE_STATE")
		}
		if publish {
			_, err = tx.ExecContext(ctx, db.EditSliceUpdate, id)
		} else {
			_, err = tx.ExecContext(ctx, db.EditSliceUpdate2, title, body, id)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.Slice(ctx, u, id)
}
