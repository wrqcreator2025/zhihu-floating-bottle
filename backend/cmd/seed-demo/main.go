// seed-demo creates an idempotent local data set and short-lived development tokens.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"driftbottle/internal/auth"
	"driftbottle/internal/config"
	"driftbottle/internal/domain"
	repository "driftbottle/internal/repository/mysql"
)

type demoUser struct {
	Subject, Label string
}

func main() {
	if os.Getenv("APP_ENV") == "production" || len(os.Getenv("AUTH_SIGNING_KEY")) < 32 {
		fail(fmt.Errorf("development only: set AUTH_SIGNING_KEY to at least 32 bytes"))
	}
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := repository.Open(ctx, cfg.DSN)
	if err != nil {
		fail(err)
	}
	defer store.DB.Close()

	users := []demoUser{{"demo-alice", "Alice（抛瓶者）"}, {"demo-bob", "Bob（待接瓶）"}, {"demo-carol", "Carol（待接瓶）"}, {"demo-dave", "Dave（空账号）"}}
	ids := map[string]string{}
	for _, user := range users {
		id, e := repository.User(ctx, store.DB, user.Subject)
		if e != nil {
			fail(e)
		}
		ids[user.Subject] = id
	}

	err = store.Tx(ctx, func(tx *sql.Tx) error {
		bobExperience, err := ensureExperience(ctx, tx, ids["demo-bob"], "从职业低谷重新出发", "我曾经在转行期长时间没有方向，后来通过小项目和同行交流重新建立节奏。")
		if err != nil {
			return err
		}
		if _, err = ensureExperience(ctx, tx, ids["demo-carol"], "搬到陌生城市后建立新生活", "刚搬家时几乎不认识任何人，我从固定的周末活动开始慢慢找到归属感。"); err != nil {
			return err
		}
		bottles := []struct{ raw, title string }{
			{"最近准备转行，投了很多简历却一直没有回音。", "转行期的迷茫"},
			{"来到新城市半年，还没有找到可以聊天的朋友。", "陌生城市的孤独"},
			{"一个项目结束后突然失去目标，想听听别人怎么度过这种阶段。", "完成目标后的空白"},
		}
		var firstBottle string
		for i, bottle := range bottles {
			id, e := ensureBottle(ctx, tx, ids["demo-alice"], bottle.raw, bottle.title)
			if e != nil {
				return e
			}
			if i == 0 {
				firstBottle = id
			}
			if _, e = tx.ExecContext(ctx, `INSERT IGNORE INTO active_search_slots(user_id,bottle_id) VALUES (?,?)`, ids["demo-alice"], id); e != nil {
				return e
			}
		}
		invitation, err := ensureInvitation(ctx, tx, firstBottle, ids["demo-bob"], bobExperience)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO notifications(id,event_key,user_id,type,resource_type,resource_id,title) VALUES (?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE title=VALUES(title)`, domain.ID(), "demo-invitation-bob", ids["demo-bob"], "invitation_received", "invitation", invitation, "有一只瓶子漂到了你身边")
		return err
	})
	if err != nil {
		fail(err)
	}

	issuer := auth.Auth{Key: []byte(cfg.AuthKey), Issuer: cfg.Issuer, Audience: cfg.Audience}
	fmt.Println("测试数据已就绪（Token 24 小时有效）：")
	for _, user := range users {
		token, err := issuer.Issue(user.Subject, 24*time.Hour)
		if err != nil {
			fail(err)
		}
		fmt.Printf("%s\nsubject: %s\nuserId: usr_%s\ntoken: %s\n\n", user.Label, user.Subject, ids[user.Subject], token)
	}
}

func ensureExperience(ctx context.Context, tx *sql.Tx, owner, title, body string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM experiences WHERE owner_id=? AND title=? LIMIT 1`, owner, title).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	id = domain.ID()
	_, err = tx.ExecContext(ctx, `INSERT INTO experiences(id,owner_id,title,body,confirmed_by_user,receive_open,disclosure,source) VALUES (?,?,?,?,1,1,JSON_OBJECT('title',true,'body',true),'manual')`, id, owner, title, body)
	return id, err
}

func ensureBottle(ctx context.Context, tx *sql.Tx, owner, raw, title string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM bottles WHERE owner_id=? AND episode_raw=? LIMIT 1`, owner, raw).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE bottles SET episode_title=?,episode_confirmed=?,target_hint='希望找到经历过相似阶段的人',target_rules=JSON_OBJECT('requiredExperiences',JSON_ARRAY(),'preferredExperiences',JSON_ARRAY(),'viewpointPreferences',JSON_ARRAY()),status='searching',failure_reason=NULL,search_round=GREATEST(search_round,1),launched_at=COALESCE(launched_at,UTC_TIMESTAMP(6)) WHERE id=?`, title, raw, id)
		return id, err
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	id = domain.ID()
	_, err = tx.ExecContext(ctx, `INSERT INTO bottles(id,owner_id,episode_raw,episode_title,episode_confirmed,target_hint,target_rules,search_round,status,launched_at) VALUES (?,?,?,?,?,'希望找到经历过相似阶段的人',JSON_OBJECT('requiredExperiences',JSON_ARRAY(),'preferredExperiences',JSON_ARRAY(),'viewpointPreferences',JSON_ARRAY()),1,'searching',UTC_TIMESTAMP(6))`, id, owner, raw, title, raw)
	return id, err
}

func ensureInvitation(ctx context.Context, tx *sql.Tx, bottle, recipient, experience string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM match_invitations WHERE bottle_id=? AND recipient_id=?`, bottle, recipient).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE match_invitations SET status='pending',expires_at=UTC_TIMESTAMP(6)+INTERVAL 72 HOUR WHERE id=?`, id)
		return id, err
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	id = domain.ID()
	_, err = tx.ExecContext(ctx, `INSERT INTO match_invitations(id,bottle_id,search_round,recipient_id,matched_experience_id,experience_snapshot,reason,status,expires_at) VALUES (?,?,1,?,?,JSON_OBJECT('title','从职业低谷重新出发'),'你的经历与这只瓶子的问题相似','pending',UTC_TIMESTAMP(6)+INTERVAL 72 HOUR)`, id, bottle, recipient, experience)
	return id, err
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
