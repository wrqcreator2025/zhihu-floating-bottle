package config

import (
	"strings"
	"testing"
)

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"HTTP_ADDR", "PORT", "MYSQL_DSN", "MYSQLHOST", "MYSQLPORT", "MYSQLUSER", "MYSQLPASSWORD", "MYSQLDATABASE", "AI_BASE_URL", "AI_API_KEY", "AI_MODEL", "ZHIHU_ACCESS_SECRET"} {
		t.Setenv(key, "")
	}
}

func TestLoadUsesZhihuDirectAnswerWhenAccessSecretIsConfigured(t *testing.T) {
	cleanEnv(t)
	t.Setenv("MYSQL_DSN", "user:pass@tcp(localhost:3306)/db")
	t.Setenv("ZHIHU_ACCESS_SECRET", "zhihu-secret")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AIURL != "https://developer.zhihu.com/v1" || c.AIModel != "zhida-fast-1p5" || c.AIKey != "zhihu-secret" {
		t.Fatalf("unexpected Zhihu AI defaults: URL=%q model=%q key-set=%t", c.AIURL, c.AIModel, c.AIKey != "")
	}
}

func TestExplicitAIProviderOverridesZhihuDefaults(t *testing.T) {
	cleanEnv(t)
	t.Setenv("MYSQL_DSN", "user:pass@tcp(localhost:3306)/db")
	t.Setenv("ZHIHU_ACCESS_SECRET", "zhihu-secret")
	t.Setenv("AI_BASE_URL", "https://models.example/v1")
	t.Setenv("AI_MODEL", "custom-model")
	t.Setenv("AI_API_KEY", "custom-key")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AIURL != "https://models.example/v1" || c.AIModel != "custom-model" || c.AIKey != "custom-key" {
		t.Fatalf("explicit AI provider was overwritten: %+v", c)
	}
}

func TestLoadRejectsPartialAIOverride(t *testing.T) {
	cleanEnv(t)
	t.Setenv("MYSQL_DSN", "user:pass@tcp(localhost:3306)/db")
	t.Setenv("ZHIHU_ACCESS_SECRET", "zhihu-secret")
	t.Setenv("AI_MODEL", "custom-model")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("expected partial AI override error, got %v", err)
	}
}

func TestLoadUsesLocalDefaults(t *testing.T) {
	cleanEnv(t)
	t.Setenv("MYSQL_DSN", "user:pass@tcp(localhost:3306)/db")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != "127.0.0.1:8080" {
		t.Fatalf("Addr = %q", c.Addr)
	}
}

func TestLoadUsesRailwayPortAndMySQLVariables(t *testing.T) {
	cleanEnv(t)
	t.Setenv("PORT", "4312")
	t.Setenv("MYSQLHOST", "mysql.railway.internal")
	t.Setenv("MYSQLPORT", "3306")
	t.Setenv("MYSQLUSER", "app")
	t.Setenv("MYSQLPASSWORD", "p@ss:/word")
	t.Setenv("MYSQLDATABASE", "railway")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != "0.0.0.0:4312" {
		t.Fatalf("Addr = %q", c.Addr)
	}
	if !strings.Contains(c.DSN, "p@ss:/word") || !strings.Contains(c.DSN, "mysql.railway.internal:3306") || !strings.Contains(c.DSN, "/railway?") {
		t.Fatalf("DSN did not preserve Railway connection values: %q", c.DSN)
	}
}

func TestHTTPAddrOverridesRailwayPort(t *testing.T) {
	cleanEnv(t)
	t.Setenv("HTTP_ADDR", "127.0.0.1:9000")
	t.Setenv("PORT", "4312")
	t.Setenv("MYSQL_DSN", "user:pass@tcp(localhost:3306)/db")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != "127.0.0.1:9000" {
		t.Fatalf("Addr = %q", c.Addr)
	}
}

func TestLoadRejectsInvalidRailwayPort(t *testing.T) {
	cleanEnv(t)
	t.Setenv("PORT", "70000")
	t.Setenv("MYSQL_DSN", "user:pass@tcp(localhost:3306)/db")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid PORT to fail")
	}
}
