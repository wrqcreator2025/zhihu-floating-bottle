package app

import (
	"context"
	"crypto/cipher"
	"os"

	"driftbottle/internal/ai"
	"driftbottle/internal/config"
	db "driftbottle/internal/repository/mysql"
	"driftbottle/internal/service"
	"driftbottle/internal/zhihu"
)

func Open(ctx context.Context) (config.Config, *service.Service, cipher.AEAD, error) {
	c, err := config.Load()
	if err != nil {
		return c, nil, nil, err
	}
	var crypt cipher.AEAD
	if key := os.Getenv("ZHIHU_TOKEN_ENCRYPTION_KEY"); key != "" {
		crypt, err = zhihu.Crypt(key)
		if err != nil {
			return c, nil, nil, err
		}
	}
	store, err := db.Open(ctx, c.DSN)
	if err != nil {
		return c, nil, nil, err
	}
	if err = store.CheckSchema(ctx); err != nil {
		store.DB.Close()
		return c, nil, nil, err
	}
	s := &service.Service{Store: store, AI: ai.New(c.AIURL, c.AIKey, c.AIModel), Zhihu: zhihu.New(c.ZhihuSecret, c.AppID, c.AppKey, c.Redirect), AttemptLimit: c.AttemptLimit, SearchBudget: c.SearchBudget, ActiveBottleLimit: c.ActiveBottleLimit}
	return c, s, crypt, nil
}
