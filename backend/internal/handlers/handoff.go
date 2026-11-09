package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/secretbox"
)

// connectionHandoff is a one-time warehouse credential the analyser agent
// redeems. The chat JSON carries only the id, never the password.
type connectionHandoff struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	expires  time.Time
}

func (h *Handler) putHandoff(c connectionHandoff) string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	c.expires = time.Now().Add(2 * time.Minute)
	h.handoffMu.Lock()
	if h.handoffs == nil {
		h.handoffs = map[string]connectionHandoff{}
	}
	h.handoffs[id] = c
	h.handoffMu.Unlock()
	return id
}

func (h *Handler) takeHandoff(id string) (connectionHandoff, bool) {
	h.handoffMu.Lock()
	defer h.handoffMu.Unlock()
	c, ok := h.handoffs[id]
	if !ok {
		return connectionHandoff{}, false
	}
	delete(h.handoffs, id)
	if time.Now().After(c.expires) {
		return connectionHandoff{}, false
	}
	return c, true
}

// MigratePlaintextSecrets rewrites legacy plaintext password_enc values.
func (h *Handler) MigratePlaintextSecrets() (int, error) {
	rows, err := h.ctrl.Query(`SELECT connection_id, password_enc FROM data_connections WHERE password_enc <> ''`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id int64
		var stored string
		if err := rows.Scan(&id, &stored); err != nil {
			return n, err
		}
		if secretbox.IsSealed(stored) {
			continue
		}
		sealed, err := h.box.Seal(stored)
		if err != nil {
			return n, err
		}
		if _, err := h.ctrl.Exec(`UPDATE data_connections SET password_enc=? WHERE connection_id=?`, sealed, id); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

// RedeemHandoff is called by the local agent (X-Internal-Token = JWT secret).
func (h *Handler) RedeemHandoff(w http.ResponseWriter, r *http.Request) {
	got := r.Header.Get("X-Internal-Token")
	want := h.cfg.JWTSecret
	if len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		writeErr(w, 401, "unauthorized")
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeErr(w, 400, "id required")
		return
	}
	c, ok := h.takeHandoff(body.ID)
	if !ok {
		writeErr(w, 404, "handoff expired or already used")
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"host": c.Host, "port": c.Port, "database": c.Database,
		"username": c.Username, "password": c.Password,
	})
}
