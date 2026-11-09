package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Schema evolution: widen data_connections to support multiple drivers.
// ---------------------------------------------------------------------------
var connSchemaOnce sync.Once

func (h *Handler) ensureConnectionSchema() {
	connSchemaOnce.Do(func() {
		// db_type was an ENUM('MYSQL'); widen it so new drivers can be stored.
		if _, err := h.ctrl.Exec(
			`ALTER TABLE data_connections MODIFY db_type VARCHAR(20) NOT NULL DEFAULT 'MYSQL'`); err != nil {
			log.Printf("ensure db_type widen: %v", err)
		}
		h.addColumnIfMissing("data_connections", "http_path", "VARCHAR(512) NULL")
		h.addColumnIfMissing("data_connections", "extra_config", "JSON NULL")
		// Databricks tokens/secrets are longer than the original 512 cap.
		if _, err := h.ctrl.Exec(
			`ALTER TABLE data_connections MODIFY password_enc VARCHAR(2048) NOT NULL DEFAULT ''`); err != nil {
			log.Printf("ensure password_enc widen: %v", err)
		}
	})
}

func (h *Handler) addColumnIfMissing(table, col, ddl string) {
	var n int
	_ = h.ctrl.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`, table, col).Scan(&n)
	if n == 0 {
		if _, err := h.ctrl.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col, ddl)); err != nil {
			log.Printf("add column %s.%s: %v", table, col, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Databricks connectivity test via the SQL Statement Execution REST API.
// Supports a Personal Access Token (PAT) or OAuth M2M (service principal).
// ---------------------------------------------------------------------------
type dbxExtra struct {
	WarehouseID string `json:"warehouseId"`
	AuthType    string `json:"authType"`
	ClientID    string `json:"clientId"`
	WorkspaceID string `json:"workspaceId"`
	OauthURL    string `json:"oauthUrl"`
	Catalog     string `json:"catalog"`
	Schema      string `json:"schema"`
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if i := strings.IndexAny(host, "/?"); i >= 0 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, "/")
}

func warehouseFromPath(httpPath string) string {
	if i := strings.LastIndex(httpPath, "/warehouses/"); i >= 0 {
		return strings.Trim(httpPath[i+len("/warehouses/"):], "/")
	}
	return ""
}

// testDatabricks returns (ok, status, message).
func (h *Handler) testDatabricks(host, httpPath, username, secret, extraJSON string) (bool, string, string) {
	var ex dbxExtra
	if extraJSON != "" {
		_ = json.Unmarshal([]byte(extraJSON), &ex)
	}
	host = normalizeHost(host)
	if host == "" {
		return false, "failed", "Databricks host is missing."
	}
	base := "https://" + host

	authType := strings.ToLower(strings.TrimSpace(ex.AuthType))
	if authType == "" {
		if strings.TrimSpace(ex.ClientID) != "" {
			authType = "oauth"
		} else {
			authType = "token"
		}
	}

	client := &http.Client{Timeout: 25 * time.Second}

	var bearer string
	switch authType {
	case "oauth":
		clientID := strings.TrimSpace(ex.ClientID)
		if clientID == "" {
			clientID = strings.TrimSpace(username)
		}
		if clientID == "" || strings.TrimSpace(secret) == "" {
			return false, "needs_credentials",
				"OAuth service principal requires a client ID and client secret. Only the service-principal name was provided — paste its OAuth secret (or switch to a Personal Access Token)."
		}
		tok, err := databricksOAuthToken(client, base, clientID, secret)
		if err != nil {
			return false, "failed", "OAuth token request failed: " + err.Error()
		}
		bearer = tok
	default: // token / PAT
		bearer = strings.TrimSpace(secret)
		if bearer == "" {
			return false, "needs_credentials",
				"Databricks needs a Personal Access Token (or service-principal OAuth secret) to authenticate. Host and warehouse are configured — paste a token to run the test."
		}
	}

	warehouse := strings.TrimSpace(ex.WarehouseID)
	if warehouse == "" {
		warehouse = warehouseFromPath(httpPath)
	}
	if warehouse == "" {
		return false, "failed", "Warehouse ID is missing (expected in extra config or the httpPath)."
	}

	payload := map[string]interface{}{
		"warehouse_id": warehouse,
		"statement":    "SELECT 1",
		"wait_timeout": "30s",
	}
	if c := strings.TrimSpace(ex.Catalog); c != "" {
		payload["catalog"] = c
	}
	if s := strings.TrimSpace(ex.Schema); s != "" {
		payload["schema"] = s
	}
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, base+"/api/2.0/sql/statements", bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return false, "failed", "Could not reach Databricks: " + err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, "failed", fmt.Sprintf("Authentication rejected (HTTP %d). Check the token/secret and that the service principal has access to the warehouse.", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return false, "failed", fmt.Sprintf("Databricks returned HTTP %d: %s", resp.StatusCode, snippet(body))
	}

	var out struct {
		Status struct {
			State string `json:"state"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"status"`
	}
	_ = json.Unmarshal(body, &out)
	switch out.Status.State {
	case "SUCCEEDED", "PENDING", "RUNNING":
		return true, "connected", fmt.Sprintf("Connected — SELECT 1 %s on warehouse %s.", strings.ToLower(out.Status.State), warehouse)
	case "":
		return true, "connected", "Connected to Databricks warehouse " + warehouse + "."
	default:
		msg := out.Status.Error.Message
		if msg == "" {
			msg = out.Status.State
		}
		return false, "failed", "Statement failed: " + msg
	}
}

func databricksOAuthToken(client *http.Client, base, clientID, secret string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("scope", "all-apis")
	req, _ := http.NewRequest(http.MethodPost, base+"/oidc/v1/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, secret)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("no access_token in response")
	}
	return tok.AccessToken, nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
