package service

import (
	"context"
	"crypto/cipher"
	"database/sql"
	"strings"

	"driftbottle/internal/ai"
	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
	"driftbottle/internal/zhihu"
)

type Service struct {
	Store                                         *db.Store
	AI                                            ai.Provider
	Zhihu                                         *zhihu.Client
	AttemptLimit, SearchBudget, ActiveBottleLimit int
}

func own(b domain.Bottle, u string) error {
	if b.Owner != u {
		return domain.NotFound
	}
	return nil
}
func participant(c domain.Connection, u string) error {
	if !c.Participant(u) {
		return domain.NotFound
	}
	return nil
}
func (s *Service) User(ctx context.Context, subject string) (string, error) {
	return db.User(ctx, s.Store.DB, subject)
}
func (s *Service) LoginZhihu(ctx context.Context, subject, name, avatar string, token zhihu.Token, crypt cipher.AEAD) (string, error) {
	profile, err := db.ZhihuUser(ctx, s.Store.DB, subject, name, avatar)
	if err != nil {
		return "", err
	}
	if err = s.ConnectZhihu(ctx, profile.ID, token, crypt); err != nil {
		return "", err
	}
	return profile.Subject, nil
}
func (s *Service) Profile(ctx context.Context, user string) (any, error) {
	profile, err := db.Profile(ctx, s.Store.DB, user)
	if err != nil {
		return nil, err
	}
	provider := "host"
	if strings.HasPrefix(profile.Subject, "zhihu:") {
		provider = "zhihu"
	}
	return map[string]any{"id": domain.Public("usr", profile.ID), "name": profile.Name, "avatarUrl": profile.Avatar, "provider": provider}, nil
}
func (s *Service) ownedBottle(ctx context.Context, q db.Queryer, u, id string, lock bool) (domain.Bottle, error) {
	b, e := db.Bottle(ctx, q, id, lock)
	if e == nil {
		e = own(b, u)
	}
	return b, e
}
func (s *Service) connection(ctx context.Context, q db.Queryer, u, id string, lock bool) (domain.Connection, error) {
	c, e := db.Connection(ctx, q, id, lock)
	if e == nil {
		e = participant(c, u)
	}
	return c, e
}
func affected(r sql.Result, e error) error {
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return domain.NotFound
	}
	return nil
}
