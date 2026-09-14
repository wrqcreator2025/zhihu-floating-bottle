package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"driftbottle/internal/app"
	"driftbottle/internal/auth"
	"driftbottle/internal/domain"
	"driftbottle/internal/httpapi"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	c, s, crypt, err := app.Open(ctx)
	if err != nil {
		var e *domain.Error
		if errors.As(err, &e) {
			slog.Error("startup failed", "code", e.Code, "message", e.Message)
		} else {
			slog.Error("startup failed; check configuration and database")
		}
		os.Exit(1)
	}
	defer s.Store.DB.Close()
	if len(c.AuthKey) < 32 {
		slog.Error("AUTH_SIGNING_KEY requires at least 32 bytes")
		os.Exit(1)
	}
	router := httpapi.New(s, httpapi.Options{Auth: auth.Auth{Key: []byte(c.AuthKey), Issuer: c.Issuer, Audience: c.Audience}, Origin: c.Origin, Origins: c.Origins, Cipher: crypt, SecureCookies: c.Env == "production"})
	server := &http.Server{Addr: c.Addr, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	slog.Info("api listening", "address", c.Addr)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("api stopped unexpectedly")
		os.Exit(1)
	}
}
