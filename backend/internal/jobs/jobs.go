package jobs

import (
	"context"
	"crypto/cipher"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
	"driftbottle/internal/service"
)

type Worker struct {
	Service *service.Service
	Cipher  cipher.AEAD
}

func (w *Worker) Claim(ctx context.Context) (*domain.Job, error) {
	var j domain.Job
	err := w.Service.Store.Tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, db.ClaimSelect).Scan(&j.ID, &j.Type, &j.Payload, &j.Attempts)
		if err != nil {
			return err
		}
		j.Attempts++
		j.Owner = domain.ID()
		_, err = tx.ExecContext(ctx, db.ClaimUpdate, j.Attempts, j.Owner, j.ID)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &j, err
}
func (w *Worker) Once(ctx context.Context) (bool, error) {
	j, err := w.Claim(ctx)
	if err != nil || j == nil {
		return false, err
	}
	slog.Info("background task started", "job_id", j.ID, "type", j.Type, "attempt", j.Attempts)
	taskCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var p domain.JobPayload
	err = json.Unmarshal(j.Payload, &p)
	if err == nil {
		switch j.Type {
		case "match_bottle":
			err = w.Service.MatchBottle(taskCtx, p)
		case "moderate_message":
			err = w.Service.ModerateMessage(taskCtx, p)
		case "generate_slice":
			err = w.Service.GenerateSlice(taskCtx, p)
		case "refresh_activity":
			err = w.Service.RefreshActivity(taskCtx, p.User, w.Cipher)
		default:
			err = domain.Fail(400, "UNKNOWN_JOB", "未知任务")
		}
	}
	return true, w.finish(ctx, *j, p, err)
}
func (w *Worker) finish(ctx context.Context, j domain.Job, p domain.JobPayload, taskErr error) error {
	return w.Service.Store.Tx(ctx, func(tx *sql.Tx) error {
		var owner, status string
		err := tx.QueryRowContext(ctx, db.FinishSelect, j.ID).Scan(&owner, &status)
		if err != nil {
			return err
		}
		if owner != j.Owner || status != "processing" {
			return nil
		}
		if taskErr == nil {
			_, err = tx.ExecContext(ctx, db.FinishUpdate, j.ID)
			return err
		}
		code := "TASK_FAILED"
		var de *domain.Error
		permanent := false
		if errors.As(taskErr, &de) {
			code = de.Code
			permanent = de.Status == 400 || de.Status == 404 || de.Status == 409 || de.Status == 422
		}
		slog.Warn("background task failed", "type", j.Type, "code", code, "attempt", j.Attempts)
		if retry, delay := retryPlan(code, j.Attempts); retry && !permanent {
			_, err = tx.ExecContext(ctx, db.FinishUpdate2, delay, code, j.ID)
			return err
		}
		_, err = tx.ExecContext(ctx, db.FinishUpdate3, code, j.ID)
		if err != nil {
			return err
		}
		switch j.Type {
		case "match_bottle":
			b, e := db.Bottle(ctx, tx, p.ID, true)
			if e != nil {
				return e
			}
			if b.Status != "searching" || b.Round != p.Round {
				return nil
			}
			_, err = tx.ExecContext(ctx, db.FinishUpdate4, p.ID)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, db.BottleActionDelete, p.ID)
			if err != nil {
				return err
			}
			return db.Notify(ctx, tx, b.Owner, "search_error", "bottle", p.ID, fmt.Sprintf("search-error:%s:%d", p.ID, p.Round), "寻找暂时遇到故障，可以稍后重试")
		case "moderate_message":
			_, err = tx.ExecContext(ctx, db.FinishUpdate5, p.ID, p.Version)
		case "generate_slice":
			_, err = tx.ExecContext(ctx, db.FinishUpdate6, p.ID)
		}
		return err
	})
}

func retryPlan(code string, attempts int) (bool, int) {
	if code == "AI_HTTP_429" {
		delay := min(7200, 60*(1<<min(max(attempts-1, 0), 7))+rand.IntN(30))
		return attempts < 12, delay
	}
	delay := min(1800, (1<<min(attempts, 10))+rand.IntN(3))
	return attempts < 5, delay
}

func (w *Worker) Recover(ctx context.Context) error {
	return w.Service.Store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, db.RecoverUpdate); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, db.RecoverSelect)
		if err != nil {
			return err
		}
		pending := []domain.JobPayload{}
		for rows.Next() {
			var p domain.JobPayload
			if err = rows.Scan(&p.ID, &p.Round); err != nil {
				rows.Close()
				return err
			}
			pending = append(pending, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, p := range pending {
			if err = db.Enqueue(ctx, tx, "match_bottle", p, fmt.Sprintf("recover:%s:%d:%s", p.ID, p.Round, time.Now().UTC().Format("200601021504"))); err != nil {
				return err
			}
		}
		rows, err = tx.QueryContext(ctx, db.RecoverSelect2)
		if err != nil {
			return err
		}
		users := []string{}
		for rows.Next() {
			var u string
			if err = rows.Scan(&u); err != nil {
				rows.Close()
				return err
			}
			users = append(users, u)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, u := range users {
			if err = db.Enqueue(ctx, tx, "refresh_activity", domain.JobPayload{User: u}, "profile:"+u+":"+time.Now().UTC().Format("2006010215")); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, db.RecoverUpdate2); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, db.RecoverDelete); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, db.RecoverDelete2)
		return err
	})
}
func (w *Worker) Run(ctx context.Context) error {
	if err := w.Recover(ctx); err != nil {
		return err
	}
	slog.Info("worker ready")
	poll := time.NewTicker(time.Second)
	defer poll.Stop()
	recoverTick := time.NewTicker(time.Minute)
	defer recoverTick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-recoverTick.C:
			if err := w.Recover(ctx); err != nil {
				slog.Error("worker recovery failed")
			}
		case <-poll.C:
			for range 20 {
				worked, err := w.Once(ctx)
				if err != nil {
					slog.Error("worker iteration failed")
					break
				}
				if !worked {
					break
				}
			}
		}
	}
}
