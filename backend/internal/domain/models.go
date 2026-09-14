package domain

import (
	"crypto/rand"
	"encoding/json"
	"time"
)

// IDs are time ordered 128-bit values encoded as 26 Crockford characters.
func ID() string {
	var b [16]byte
	n := uint64(time.Now().UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(n)
		n >>= 8
	}
	if _, err := rand.Read(b[6:]); err != nil {
		panic(err)
	}
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	var encoded [26]byte
	for i := range encoded {
		var value byte
		for j := 0; j < 5; j++ {
			bit := i*5 + j - 2
			value <<= 1
			if bit >= 0 {
				value |= (b[bit/8] >> uint(7-bit%8)) & 1
			}
		}
		encoded[i] = alphabet[value]
	}
	return string(encoded[:])
}
func Public(prefix, id string) string {
	if id == "" {
		return ""
	}
	return prefix + "_" + id
}
func JSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code }
func Fail(status int, code, message string) error {
	return &Error{Status: status, Code: code, Message: message}
}
func Conflict(code string) error { return Fail(409, code, "当前状态不允许此操作") }

var NotFound = Fail(404, "NOT_FOUND", "记录不存在")

type Target struct {
	Required   []string `json:"requiredExperiences"`
	Preferred  []string `json:"preferredExperiences"`
	Viewpoints []string `json:"viewpointPreferences"`
}

func (t *Target) Normalize() {
	if t.Required == nil {
		t.Required = []string{}
	}
	if t.Preferred == nil {
		t.Preferred = []string{}
	}
	if t.Viewpoints == nil {
		t.Viewpoints = []string{}
	}
}

type Bottle struct {
	ID        string     `json:"id"`
	Owner     string     `json:"-"`
	Raw       string     `json:"-"`
	Title     string     `json:"-"`
	Confirmed bool       `json:"-"`
	Hint      string     `json:"-"`
	Target    Target     `json:"-"`
	Version   int        `json:"contentVersion"`
	Round     int        `json:"searchRound"`
	Status    string     `json:"status"`
	Failure   *string    `json:"failureReason,omitempty"`
	Created   time.Time  `json:"createdAt"`
	Updated   time.Time  `json:"updatedAt"`
	Launched  *time.Time `json:"launchedAt"`
}

func (b Bottle) View() map[string]any {
	b.Target.Normalize()
	target := map[string]any{"hintText": b.Hint, "requiredExperiences": b.Target.Required, "preferredExperiences": b.Target.Preferred, "viewpointPreferences": b.Target.Viewpoints}
	return map[string]any{"id": Public("btl", b.ID), "ownerRole": "sender", "episode": map[string]any{"rawText": b.Raw, "title": b.Title, "confirmed": b.Confirmed}, "target": target, "contentVersion": b.Version, "searchRound": b.Round, "status": b.Status, "failureReason": b.Failure, "createdAt": b.Created, "updatedAt": b.Updated, "launchedAt": b.Launched}
}

type Disclosure struct {
	Summary   bool `json:"summary"`
	TimeRange bool `json:"timeRange"`
	Domain    bool `json:"domain"`
}
type Experience struct {
	ID         string     `json:"id"`
	Owner      string     `json:"-"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Confirmed  bool       `json:"confirmedByUser"`
	Open       bool       `json:"receiveOpen"`
	Disclosure Disclosure `json:"disclosure"`
	Source     string     `json:"source"`
	Updated    time.Time  `json:"updatedAt"`
}

func (e Experience) Snapshot() map[string]any {
	v := map[string]any{"disclosure": e.Disclosure}
	if e.Disclosure.Summary {
		v["title"] = e.Title
	}
	return v
}

type Connection struct {
	ID, BottleID, InvitationID, Seeker, Responder, Status string
	Created                                               time.Time
	Closed                                                *time.Time
}

func (c Connection) Participant(u string) bool { return u == c.Seeker || u == c.Responder }
func (c Connection) Other(u string) string {
	if u == c.Seeker {
		return c.Responder
	}
	return c.Seeker
}

type Message struct {
	ID      string    `json:"id"`
	Body    string    `json:"body"`
	Kind    string    `json:"kind"`
	Status  string    `json:"deliveryStatus"`
	Version int       `json:"contentVersion"`
	Role    string    `json:"senderRole"`
	Created time.Time `json:"createdAt"`
}
type Job struct {
	ID, Type, Owner string
	Payload         json.RawMessage
	Attempts        int
}
type JobPayload struct {
	ID      string `json:"id"`
	Version int    `json:"version,omitempty"`
	Round   int    `json:"round,omitempty"`
	User    string `json:"user,omitempty"`
}
