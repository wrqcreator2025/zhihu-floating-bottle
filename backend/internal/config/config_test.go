package config

import (
	"strings"
	"testing"
)

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"HTTP_ADDR", "PORT", "MYSQL_DSN", "MYSQLHOST", "MYSQLPORT", "MYSQLUSER", "MYSQLPASSWORD", "MYSQLDATABASE"} {
		t.Setenv(key, "")
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
