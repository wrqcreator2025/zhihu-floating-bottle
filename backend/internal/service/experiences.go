package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
)

type ExperienceInput struct {
	Title      *string            `json:"title"`
	Body       *string            `json:"body"`
	Confirmed  *bool              `json:"confirmedByUser"`
	Open       *bool              `json:"receiveOpen"`
	Disclosure *domain.Disclosure `json:"disclosure"`
}

func (s *Service) Experiences(ctx context.Context, u string) (any, error) {
	rows, err := s.Store.DB.QueryContext(ctx, db.ExperiencesSelect, u)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Experience{}
	for rows.Next() {
		var e domain.Experience
		var raw []byte
		if err = rows.Scan(&e.ID, &e.Title, &e.Body, &e.Confirmed, &e.Open, &raw, &e.Source, &e.Updated); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &e.Disclosure); err != nil {
			return nil, err
		}
		e.ID = domain.Public("exp", e.ID)
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Service) SaveExperience(ctx context.Context, u, id string, in ExperienceInput) (any, error) {
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		if id == "" {
			id = domain.ID()
			open := false
			if in.Open != nil {
				open = *in.Open
			}
			d := domain.Disclosure{}
			if in.Disclosure != nil {
				d = *in.Disclosure
			}
			_, err := tx.ExecContext(ctx, db.SaveExperienceInsert, id, u, *in.Title, *in.Body, open, domain.JSON(d))
			return err
		}
		var title, body string
		var confirmed, open bool
		var raw []byte
		err := tx.QueryRowContext(ctx, db.SaveExperienceSelect, id, u).Scan(&title, &body, &confirmed, &open, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.NotFound
		}
		if err != nil {
			return err
		}
		if in.Title != nil {
			title = *in.Title
		}
		if in.Body != nil {
			body = *in.Body
		}
		if in.Open != nil {
			open = *in.Open
		}
		if in.Confirmed != nil {
			confirmed = *in.Confirmed
		}
		if !confirmed {
			return domain.Conflict("EXPERIENCE_CONFIRMATION_REQUIRED")
		}
		if in.Disclosure != nil {
			raw = []byte(domain.JSON(in.Disclosure))
		}
		_, err = tx.ExecContext(ctx, db.SaveExperienceUpdate, title, body, confirmed, open, raw, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	var e domain.Experience
	var raw []byte
	err = s.Store.DB.QueryRowContext(ctx, db.SaveExperienceSelect2, id).Scan(&e.ID, &e.Title, &e.Body, &e.Confirmed, &e.Open, &raw, &e.Source, &e.Updated)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &e.Disclosure)
	e.ID = domain.Public("exp", e.ID)
	return e, err
}
func (s *Service) DeleteExperience(ctx context.Context, u, id string) error {
	return affected(s.Store.DB.ExecContext(ctx, db.DeleteExperienceDelete, id, u))
}
