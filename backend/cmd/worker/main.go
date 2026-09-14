package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"driftbottle/internal/app"
	"driftbottle/internal/domain"
	"driftbottle/internal/jobs"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	_, s, crypt, err := app.Open(ctx)
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
	w := jobs.Worker{Service: s, Cipher: crypt}
	if err = w.Run(ctx); err != nil {
		slog.Error("worker stopped unexpectedly")
		os.Exit(1)
	}
}
