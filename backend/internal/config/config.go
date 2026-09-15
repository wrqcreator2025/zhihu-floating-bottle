package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

type Config struct {
	Env, Addr, DSN, AuthKey, Issuer, Audience, Origin, Origins, AIURL, AIKey, AIModel, ZhihuSecret, AppID, AppKey, Redirect string
	SearchBudget, AttemptLimit, ActiveBottleLimit                                                                           int
}

func Load() (Config, error) {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		if port := os.Getenv("PORT"); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return Config{}, fmt.Errorf("PORT must be between 1 and 65535")
			}
			addr = net.JoinHostPort("0.0.0.0", port)
		} else {
			addr = "127.0.0.1:8080"
		}
	}
	c := Config{Env: env("APP_ENV", "development"), Addr: addr, DSN: railwayDSN(), AuthKey: os.Getenv("AUTH_SIGNING_KEY"), Issuer: env("AUTH_ISSUER", "drift-bottle"), Audience: env("AUTH_AUDIENCE", "drift-bottle-web"), Origin: env("CORS_ORIGIN", "http://localhost:5173"), Origins: os.Getenv("CORS_ORIGINS"), AIURL: os.Getenv("AI_BASE_URL"), AIKey: os.Getenv("AI_API_KEY"), AIModel: os.Getenv("AI_MODEL"), ZhihuSecret: os.Getenv("ZHIHU_ACCESS_SECRET"), AppID: os.Getenv("ZHIHU_OAUTH_APP_ID"), AppKey: os.Getenv("ZHIHU_OAUTH_APP_KEY"), Redirect: os.Getenv("ZHIHU_OAUTH_REDIRECT_URI"), SearchBudget: 500, AttemptLimit: 5, ActiveBottleLimit: 3}
	// A Zhihu Access Secret grants access to both Search and Direct Answer.
	// Explicit AI_* values always win, so another compatible provider remains usable.
	if c.AIURL == "" && c.AIModel == "" && c.AIKey == "" && c.ZhihuSecret != "" {
		c.AIURL = "https://developer.zhihu.com/v1"
		c.AIModel = "zhida-fast-1p5"
		c.AIKey = c.ZhihuSecret
	}
	aiValues := 0
	for _, value := range []string{c.AIURL, c.AIModel, c.AIKey} {
		if value != "" {
			aiValues++
		}
	}
	if aiValues != 0 && aiValues != 3 {
		return c, fmt.Errorf("AI_BASE_URL, AI_MODEL and AI_API_KEY must be configured together")
	}
	var err error
	if v := os.Getenv("ZHIHU_SEARCH_DAILY_BUDGET"); v != "" {
		c.SearchBudget, err = strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("invalid search budget")
		}
	}
	if c.SearchBudget < 1 || c.SearchBudget > 5000 {
		return c, fmt.Errorf("search budget must be 1..5000")
	}
	if v := os.Getenv("ACTIVE_BOTTLE_LIMIT"); v != "" {
		c.ActiveBottleLimit, err = strconv.Atoi(v)
		if err != nil || c.ActiveBottleLimit < 3 || c.ActiveBottleLimit > 20 {
			return c, fmt.Errorf("ACTIVE_BOTTLE_LIMIT must be 3..20")
		}
	}
	if c.DSN == "" {
		return c, fmt.Errorf("MYSQL_DSN or Railway MYSQLHOST/MYSQLUSER/MYSQLDATABASE variables are required")
	}
	return c, nil
}

func railwayDSN() string {
	if dsn := os.Getenv("MYSQL_DSN"); dsn != "" {
		return dsn
	}
	host, user, pass, database := os.Getenv("MYSQLHOST"), os.Getenv("MYSQLUSER"), os.Getenv("MYSQLPASSWORD"), os.Getenv("MYSQLDATABASE")
	if host == "" || user == "" || database == "" {
		return ""
	}
	c := mysql.NewConfig()
	c.User, c.Passwd = user, pass
	c.Net = "tcp"
	c.Addr = net.JoinHostPort(host, env("MYSQLPORT", "3306"))
	c.DBName = database
	c.ParseTime = true
	c.Loc = time.UTC
	c.Timeout, c.ReadTimeout, c.WriteTimeout = 5*time.Second, 10*time.Second, 10*time.Second
	c.Params = map[string]string{"charset": "utf8mb4", "time_zone": "'+00:00'"}
	return c.FormatDSN()
}

func env(k, d string) string {
	if s := os.Getenv(k); s != "" {
		return s
	}
	return d
}
