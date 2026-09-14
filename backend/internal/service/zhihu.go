package service

import (
	"context"
	"crypto/cipher"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"driftbottle/internal/ai"
	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
	"driftbottle/internal/zhihu"
)

func hash(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func today() string {
	return time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
}
func (s *Service) Search(ctx context.Context, u, query string) (zhihu.SearchResult, error) {
	var out zhihu.SearchResult
	if s.Zhihu.Secret == "" {
		return out, domain.Fail(503, "ZHIHU_UNAVAILABLE", "知乎接口尚未配置")
	}
	key := hash("zhihu_search:" + query)
	var raw []byte
	err := s.Store.DB.QueryRowContext(ctx, db.SearchSelect, key).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &out)
		return out, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	err = s.Store.Tx(ctx, func(tx *sql.Tx) error {
		account := hash(s.Zhihu.Secret)
		date := today()
		_, e := tx.ExecContext(ctx, db.SearchInsert, account, date)
		if e != nil {
			return e
		}
		r, e := tx.ExecContext(ctx, db.SearchUpdate, account, date, s.SearchBudget)
		if e != nil {
			return e
		}
		n, e := r.RowsAffected()
		if e != nil {
			return e
		}
		if n == 0 {
			return domain.Fail(429, "ZHIHU_SEARCH_DAILY_LIMIT", "今日知乎搜索预算已用完")
		}
		_, e = tx.ExecContext(ctx, db.SearchInsert2, u, date)
		return e
	})
	if err != nil {
		return out, err
	}
	out, err = s.Zhihu.Search(ctx, query)
	if err != nil {
		return out, err
	}
	_, err = s.Store.DB.ExecContext(ctx, db.SearchInsert3, key, domain.JSON(out))
	return out, err
}
func (s *Service) IntegrationStatus(ctx context.Context, u string) (any, error) {
	_, status, err := s.Activity(ctx, u)
	if err != nil {
		return nil, err
	}
	var used int
	err = s.Store.DB.QueryRowContext(ctx, db.IntegrationStatusSelect, hash(s.Zhihu.Secret), today()).Scan(&used)
	if err != nil {
		return nil, err
	}
	var integration string
	var synced *time.Time
	var expired bool
	err = s.Store.DB.QueryRowContext(ctx, db.IntegrationStatusSelect2, u).Scan(&integration, &synced, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		integration = "disconnected"
		err = nil
	}
	if err != nil {
		return nil, err
	}
	if expired && integration == "connected" {
		integration = "reauthorization_required"
	}
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Now().In(loc)
	reset := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	sources := []string{}
	if status != "unavailable" {
		sources = []string{"public_content", "followees"}
	}
	return map[string]any{"status": integration, "activityProfileAvailable": status != "unavailable", "activityStatus": status, "activitySources": sources, "lastActivitySyncAt": synced, "followFeedAvailable": false, "searchAvailable": s.Zhihu.Secret != "" && used < s.SearchBudget, "effectiveDailyLimit": s.SearchBudget, "usedToday": used, "resetsAt": reset}, nil
}
func (s *Service) ConnectZhihu(ctx context.Context, u string, t zhihu.Token, a cipher.AEAD) error {
	raw, err := zhihu.Encrypt(a, u, t.Value)
	if err != nil {
		return err
	}
	return s.Store.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, db.ConnectZhihuInsert, u, raw, t.Expires)
		if err != nil {
			return err
		}
		return db.Enqueue(ctx, tx, "refresh_activity", domain.JobPayload{User: u}, "")
	})
}
func (s *Service) DisconnectZhihu(ctx context.Context, u string) error {
	return s.Store.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, db.DisconnectZhihuUpdate, u)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, db.DisconnectZhihuDelete, u)
		return err
	})
}
func (s *Service) RefreshActivity(ctx context.Context, u string, a cipher.AEAD) error {
	if a == nil {
		return domain.Fail(503, "ZHIHU_UNAVAILABLE", "授权存储尚未配置")
	}
	var raw []byte
	var expires time.Time
	err := s.Store.DB.QueryRowContext(ctx, db.RefreshActivitySelect, u).Scan(&raw, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !expires.After(time.Now()) {
		_, err = s.Store.DB.ExecContext(ctx, db.RefreshActivityUpdate, u)
		return err
	}
	token, err := zhihu.Decrypt(a, u, raw)
	if err != nil {
		return err
	}
	contents, err := s.Zhihu.Contents(ctx, token, "0")
	if err != nil {
		return s.activityError(ctx, u, err)
	}
	followees, err := s.Zhihu.Followees(ctx, token, "0")
	if err != nil {
		return s.activityError(ctx, u, err)
	}
	texts := []string{}
	for _, item := range contents.Items {
		texts = append(texts, item.Title+" "+item.Summary)
	}
	headlines := []string{}
	for _, item := range followees.Items {
		headlines = append(headlines, item.Headline)
	}
	var profile ai.Profile
	if err = s.AI.Run(ctx, "profile", map[string]any{"contents": texts, "followeeHeadlines": headlines}, &profile); err != nil {
		return err
	}
	if len(profile.Topics) > 20 {
		profile.Topics = profile.Topics[:20]
	}
	return s.Store.Tx(ctx, func(tx *sql.Tx) error {
		var current []byte
		err := tx.QueryRowContext(ctx, db.RefreshActivitySelect2, u).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if string(current) != string(raw) {
			return nil
		}
		_, err = tx.ExecContext(ctx, db.RefreshActivityInsert, u, domain.JSON(profile))
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, db.RefreshActivityUpdate2, u)
		return err
	})
}
func (s *Service) activityError(ctx context.Context, u string, err error) error {
	var e *domain.Error
	if errors.As(err, &e) && e.Code == "ZHIHU_REAUTHORIZATION_REQUIRED" {
		_, updateErr := s.Store.DB.ExecContext(ctx, db.RefreshActivityUpdate, u)
		if updateErr != nil {
			return updateErr
		}
	}
	return err
}
