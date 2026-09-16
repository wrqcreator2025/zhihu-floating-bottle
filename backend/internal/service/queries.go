package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"driftbottle/internal/domain"
	db "driftbottle/internal/repository/mysql"
)

func (s *Service) Home(ctx context.Context, u string) (any, error) {
	var pending, unread, sent, received, experiences, unreadInvitations int
	err := s.Store.DB.QueryRowContext(ctx, db.HomeSelect, u, u, u, u, u, u).Scan(&pending, &unread, &sent, &received, &experiences, &unreadInvitations)
	if err != nil {
		return nil, err
	}
	rows, err := s.Store.DB.QueryContext(ctx, db.HomeActiveBottles, u)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	active := []any{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		b, e := s.ownedBottle(ctx, s.Store.DB, u, id, false)
		if e != nil {
			return nil, e
		}
		active = append(active, b.View())
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"activeBottles": active, "activeBottleLimit": s.ActiveBottleLimit, "pendingInvitationCount": pending, "unreadInvitationCount": unreadInvitations, "unreadReplyCount": unread, "cabinet": map[string]int{"sentCount": sent, "receivedCount": received}, "experienceCount": experiences}, nil
}
func (s *Service) Cabinet(ctx context.Context, u, direction, cursor string, limit int) ([]any, *string, error) {
	query := db.CabinetSelect
	if direction == "received" {
		query = db.CabinetSelect2
	}
	rows, err := s.Store.DB.QueryContext(ctx, query, u, u, cursor, cursor, limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []any{}
	ids := []string{}
	for rows.Next() {
		var id, bid, cid, title, preview, bs, cs string
		var updated time.Time
		var unread bool
		if err = rows.Scan(&id, &bid, &cid, &title, &preview, &bs, &cs, &updated, &unread); err != nil {
			return nil, nil, err
		}
		record := "cab_sent_" + id
		if direction == "received" {
			record = "cab_received_" + id
		}
		var connectionID, connectionStatus any
		if cid != "" {
			connectionID = domain.Public("con", cid)
			connectionStatus = cs
		}
		ids = append(ids, record)
		out = append(out, map[string]any{"id": record, "direction": direction, "bottleId": domain.Public("btl", bid), "connectionId": connectionID, "title": title, "preview": preview, "bottleStatus": bs, "connectionStatus": connectionStatus, "unread": unread, "updatedAt": updated})
	}
	var next *string
	if len(out) > limit {
		v := ids[limit-1]
		next = &v
		out = out[:limit]
	}
	return out, next, rows.Err()
}
func (s *Service) CabinetDetail(ctx context.Context, u, id string) (any, error) {
	if strings.HasPrefix(id, "cab_sent_") {
		bid := strings.TrimPrefix(id, "cab_sent_")
		b, e := s.ownedBottle(ctx, s.Store.DB, u, bid, false)
		if e != nil {
			return nil, e
		}
		if b.Launched == nil {
			return nil, domain.NotFound
		}
		return s.BottleDetail(ctx, u, bid)
	}
	if strings.HasPrefix(id, "cab_received_") {
		cid := strings.TrimPrefix(id, "cab_received_")
		c, e := s.connection(ctx, s.Store.DB, u, cid, false)
		if e != nil {
			return nil, e
		}
		if c.Responder != u {
			return nil, domain.NotFound
		}
		return s.ConnectionDetail(ctx, u, cid)
	}
	return nil, domain.NotFound
}

type notification struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	ResourceType string     `json:"resourceType"`
	ResourceID   string     `json:"resourceId"`
	Title        string     `json:"title"`
	Read         *time.Time `json:"readAt"`
	Created      time.Time  `json:"createdAt"`
}

func (n *notification) public() {
	n.ID = domain.Public("ntf", n.ID)
	prefix := map[string]string{"bottle": "btl", "invitation": "inv", "connection": "con"}[n.ResourceType]
	n.ResourceID = domain.Public(prefix, n.ResourceID)
}
func (s *Service) Notifications(ctx context.Context, u, cursor string, limit int, unread bool) ([]any, *string, error) {
	rows, err := s.Store.DB.QueryContext(ctx, db.NotificationsSelect, u, unread, cursor, cursor, limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []any{}
	ids := []string{}
	for rows.Next() {
		var n notification
		if err = rows.Scan(&n.ID, &n.Type, &n.ResourceType, &n.ResourceID, &n.Title, &n.Read, &n.Created); err != nil {
			return nil, nil, err
		}
		n.public()
		ids = append(ids, n.ID)
		out = append(out, n)
	}
	var next *string
	if len(out) > limit {
		v := ids[limit-1]
		next = &v
		out = out[:limit]
	}
	return out, next, rows.Err()
}
func (s *Service) ReadNotification(ctx context.Context, u, id string) (any, error) {
	_, err := s.Store.DB.ExecContext(ctx, db.ReadNotificationUpdate, id, u)
	if err != nil {
		return nil, err
	}
	var n notification
	err = s.Store.DB.QueryRowContext(ctx, db.ReadNotificationSelect, id, u).Scan(&n.ID, &n.Type, &n.ResourceType, &n.ResourceID, &n.Title, &n.Read, &n.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.NotFound
	}
	n.public()
	return n, err
}
