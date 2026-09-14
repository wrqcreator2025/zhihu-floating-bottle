package app

import (
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"net"
	"os"

	"driftbottle/internal/ai"
	"driftbottle/internal/config"
	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
	"driftbottle/internal/service"
	"driftbottle/internal/zhihu"

	driver "github.com/go-sql-driver/mysql"
)

func Open(ctx context.Context) (config.Config, *service.Service, cipher.AEAD, error) {
	c, err := config.Load()
	if err != nil {
		return c, nil, nil, domain.Fail(503, "STARTUP_CONFIG", err.Error())
	}
	var crypt cipher.AEAD
	if key := os.Getenv("ZHIHU_TOKEN_ENCRYPTION_KEY"); key != "" {
		crypt, err = zhihu.Crypt(key)
		if err != nil {
			return c, nil, nil, domain.Fail(503, "STARTUP_ZHIHU_ENCRYPTION", err.Error())
		}
	}
	store, err := db.Open(ctx, c.DSN)
	if err != nil {
		return c, nil, nil, databaseError("STARTUP_DATABASE_CONNECT", err)
	}
	if err = store.CheckSchema(ctx); err != nil {
		store.DB.Close()
		return c, nil, nil, databaseError("STARTUP_DATABASE_SCHEMA", err)
	}
	s := &service.Service{Store: store, AI: ai.New(c.AIURL, c.AIKey, c.AIModel), Zhihu: zhihu.New(c.ZhihuSecret, c.AppID, c.AppKey, c.Redirect), AttemptLimit: c.AttemptLimit, SearchBudget: c.SearchBudget, ActiveBottleLimit: c.ActiveBottleLimit}
	return c, s, crypt, nil
}

// Report actionable diagnostics without logging DSNs or driver messages that
// may contain credentials or other connection details.
func databaseError(stage string, err error) error {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		return err
	}
	message := fmt.Sprintf("database error (%T); check MYSQL_DSN and Railway MYSQL variables", err)
	var mysqlErr *driver.MySQLError
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.As(err, &mysqlErr):
		message = fmt.Sprintf("MySQL error %d", mysqlErr.Number)
		switch mysqlErr.Number {
		case 1045:
			message += ": authentication rejected; check MYSQLUSER and MYSQLPASSWORD"
		case 1049:
			message += ": database does not exist; check MYSQLDATABASE"
		case 1044:
			message += ": database access denied; check user permissions"
		}
	case errors.As(err, &dnsErr):
		message = "database hostname lookup failed; check MYSQLHOST and private network"
	case errors.As(err, &netErr):
		message = "database network connection failed; check MYSQLHOST, MYSQLPORT and MySQL service"
		if netErr.Timeout() {
			message = "database connection timed out; check MySQL service and private network"
		}
	}
	return domain.Fail(503, stage, message)
}
