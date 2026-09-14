package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"driftbottle/internal/domain"
)

type Store struct{ DB *sql.DB }
type Queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	c, err := driver.ParseDSN(dsn)
	if err != nil {
		return nil, errors.New("invalid MYSQL_DSN")
	}
	c.ParseTime = true
	c.Loc = time.UTC
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	c.Params["time_zone"] = "'+00:00'"
	c.Timeout = 5 * time.Second
	c.ReadTimeout = 15 * time.Second
	c.WriteTimeout = 15 * time.Second
	db, err := sql.Open("mysql", c.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(3 * time.Minute)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}
func (s *Store) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func Duplicate(err error) bool {
	var e *driver.MySQLError
	return errors.As(err, &e) && e.Number == 1062
}
func User(ctx context.Context, q Queryer, subject string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, "SELECT id FROM users WHERE external_subject=?", subject).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id = domain.ID()
	_, err = q.ExecContext(ctx, "INSERT INTO users(id,external_subject) VALUES (?,?)", id, subject)
	if Duplicate(err) {
		err = q.QueryRowContext(ctx, "SELECT id FROM users WHERE external_subject=?", subject).Scan(&id)
	}
	return id, err
}

type UserProfile struct{ ID, Subject, Name, Avatar string }

func ZhihuUser(ctx context.Context, q Queryer, subject, name, avatar string) (UserProfile, error) {
	u := UserProfile{Subject: "zhihu:" + subject, Name: name, Avatar: avatar}
	err := q.QueryRowContext(ctx, `SELECT id FROM users WHERE external_subject=?`, u.Subject).Scan(&u.ID)
	if errors.Is(err, sql.ErrNoRows) {
		u.ID = domain.ID()
		_, err = q.ExecContext(ctx, `INSERT INTO users(id,external_subject,display_name,avatar_url) VALUES (?,?,?,?)`, u.ID, u.Subject, name, avatar)
	} else if err == nil {
		_, err = q.ExecContext(ctx, `UPDATE users SET display_name=?,avatar_url=? WHERE id=?`, name, avatar, u.ID)
	}
	return u, err
}

func Profile(ctx context.Context, q Queryer, id string) (UserProfile, error) {
	var u UserProfile
	err := q.QueryRowContext(ctx, `SELECT id,COALESCE(external_subject,''),COALESCE(display_name,''),COALESCE(avatar_url,'') FROM users WHERE id=?`, id).Scan(&u.ID, &u.Subject, &u.Name, &u.Avatar)
	if errors.Is(err, sql.ErrNoRows) {
		err = domain.NotFound
	}
	return u, err
}

func Bottle(ctx context.Context, q Queryer, id string, lock bool) (domain.Bottle, error) {
	var b domain.Bottle
	var target []byte
	query := `SELECT id,owner_id,episode_raw,COALESCE(episode_title,''),episode_confirmed IS NOT NULL,COALESCE(target_hint,''),COALESCE(target_rules,JSON_OBJECT()),content_version,search_round,status,failure_reason,created_at,updated_at,launched_at FROM bottles WHERE id=?`
	if lock {
		query += " FOR UPDATE"
	}
	err := q.QueryRowContext(ctx, query, id).Scan(&b.ID, &b.Owner, &b.Raw, &b.Title, &b.Confirmed, &b.Hint, &target, &b.Version, &b.Round, &b.Status, &b.Failure, &b.Created, &b.Updated, &b.Launched)
	if errors.Is(err, sql.ErrNoRows) {
		return b, domain.NotFound
	}
	if err == nil {
		err = json.Unmarshal(target, &b.Target)
	}
	b.Target.Normalize()
	return b, err
}
func Connection(ctx context.Context, q Queryer, id string, lock bool) (domain.Connection, error) {
	var c domain.Connection
	query := `SELECT id,bottle_id,invitation_id,seeker_id,responder_id,status,created_at,closed_at FROM connections WHERE id=?`
	if lock {
		query += " FOR UPDATE"
	}
	err := q.QueryRowContext(ctx, query, id).Scan(&c.ID, &c.BottleID, &c.InvitationID, &c.Seeker, &c.Responder, &c.Status, &c.Created, &c.Closed)
	if errors.Is(err, sql.ErrNoRows) {
		err = domain.NotFound
	}
	return c, err
}
func Enqueue(ctx context.Context, q Queryer, kind string, p domain.JobPayload, event string) error {
	var key any
	if event != "" {
		key = event
	}
	_, err := q.ExecContext(ctx, `INSERT INTO outbox_jobs(id,type,payload,event_key) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE event_key=VALUES(event_key)`, domain.ID(), kind, domain.JSON(p), key)
	return err
}
func Notify(ctx context.Context, q Queryer, user, kind, resource, id, event, title string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO notifications(id,event_key,user_id,type,resource_type,resource_id,title) VALUES (?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE event_key=VALUES(event_key)`, domain.ID(), event, user, kind, resource, id, title)
	return err
}
func Blocked(ctx context.Context, q Queryer, a, b string) (bool, error) {
	var v bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_blocks WHERE (blocker_id=? AND blocked_id=?) OR (blocker_id=? AND blocked_id=?))`, a, b, b, a).Scan(&v)
	return v, err
}

// CheckSchema verifies the version 2 multi-slot shape once at startup.
func (s *Store) CheckSchema(ctx context.Context) error {
	var valid bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='active_search_slots' AND index_name='PRIMARY' GROUP BY index_name HAVING COUNT(*)=2 AND SUM(column_name='user_id')=1 AND SUM(column_name='bottle_id')=1)`).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return domain.Fail(503, "DATABASE_SCHEMA_INCOMPATIBLE", "active_search_slots 需要 version 2 的多瓶寻找结构")
	}
	return nil
}
