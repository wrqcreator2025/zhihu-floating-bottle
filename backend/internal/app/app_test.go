package app

import (
	"errors"
	"net"
	"strings"
	"testing"

	"driftbottle/internal/domain"
	driver "github.com/go-sql-driver/mysql"
)

func TestDatabaseDiagnosticsDoNotExposeDriverDetails(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&driver.MySQLError{Number: 1045, Message: "secret-password"}, "authentication rejected"},
		{&net.DNSError{Name: "secret-password", Err: "secret-password"}, "hostname lookup failed"},
		{errors.New("secret-password"), "check MYSQL_DSN"},
	} {
		err := databaseError("STARTUP_DATABASE_CONNECT", tc.err)
		var diagnostic *domain.Error
		if !errors.As(err, &diagnostic) || diagnostic.Code != "STARTUP_DATABASE_CONNECT" {
			t.Fatalf("missing startup stage: %v", err)
		}
		if strings.Contains(diagnostic.Message, "secret-password") || !strings.Contains(diagnostic.Message, tc.want) {
			t.Fatalf("unexpected diagnostic: %s", diagnostic.Message)
		}
	}
}

func TestOpenReportsInvalidEncryptionKeyBeforeDatabaseConnection(t *testing.T) {
	t.Setenv("MYSQL_DSN", "user:password@tcp(localhost:3306)/db")
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("ZHIHU_SEARCH_DAILY_BUDGET", "500")
	t.Setenv("ACTIVE_BOTTLE_LIMIT", "3")
	t.Setenv("ZHIHU_TOKEN_ENCRYPTION_KEY", "invalid-key")
	_, _, _, err := Open(t.Context())
	var diagnostic *domain.Error
	if !errors.As(err, &diagnostic) || diagnostic.Code != "STARTUP_ZHIHU_ENCRYPTION" {
		t.Fatalf("unexpected diagnostic: %v", err)
	}
}
