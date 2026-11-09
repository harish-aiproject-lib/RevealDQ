package voc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// LinkedIn OAuth 2.0 (3-legged). The platform never sees the user's LinkedIn
// password: the user signs in on linkedin.com with their own account (e.g.
// Harish.aiproject.lib@gmail.com) and LinkedIn returns an access token scoped
// to the organisations that account administers.
//
// Required app products on the LinkedIn Developer Portal:
//   - Community Management API  → r_organization_social, rw_organization_admin
//   - Sign In with LinkedIn (OpenID) → openid, profile, email  (for account label)

const (
	linkedinAuthURL  = "https://www.linkedin.com/oauth/v2/authorization"
	linkedinTokenURL = "https://www.linkedin.com/oauth/v2/accessToken"
	linkedinUserInfo = "https://api.linkedin.com/v2/userinfo"
	linkedinScopes   = "openid profile email r_organization_social rw_organization_admin"
)

type oauthState struct {
	WorkspaceID int64 `json:"ws"`
	SourceID    int64 `json:"src"`
	UserID      int64 `json:"uid"`
	Exp         int64 `json:"exp"`
}

func (c Config) oauthReady() bool {
	return c.LinkedInClientID != "" && c.LinkedInClientSecret != "" && c.LinkedInRedirectURI != ""
}

// signState encodes + HMAC-signs the state so the unauthenticated callback can
// trust it.
func (c Config) signState(s oauthState) string {
	b, _ := json.Marshal(s)
	payload := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, []byte(c.JWTSecret))
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func (c Config) verifyState(raw string) (oauthState, error) {
	var s oauthState
	parts := strings.SplitN(raw, ".", 2)
	if len(parts) != 2 {
		return s, fmt.Errorf("malformed state")
	}
	mac := hmac.New(sha256.New, []byte(c.JWTSecret))
	mac.Write([]byte(parts[0]))
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(parts[1])) {
		return s, fmt.Errorf("state signature mismatch")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if time.Now().Unix() > s.Exp {
		return s, fmt.Errorf("state expired — start the connection again")
	}
	return s, nil
}

// AuthorizeURL builds the LinkedIn consent URL for a source.
func (c Config) AuthorizeURL(ws, sourceID, uid int64) (string, error) {
	if !c.oauthReady() {
		return "", fmt.Errorf("LinkedIn OAuth is not configured — set LINKEDIN_CLIENT_ID, LINKEDIN_CLIENT_SECRET and LINKEDIN_REDIRECT_URI in .env")
	}
	state := c.signState(oauthState{WorkspaceID: ws, SourceID: sourceID, UserID: uid, Exp: time.Now().Add(15 * time.Minute).Unix()})
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.LinkedInClientID)
	q.Set("redirect_uri", c.LinkedInRedirectURI)
	q.Set("state", state)
	q.Set("scope", linkedinScopes)
	if c.LinkedInAccountEmail != "" {
		q.Set("login_hint", c.LinkedInAccountEmail)
	}
	return linkedinAuthURL + "?" + q.Encode(), nil
}

type tokenResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Scope                 string `json:"scope"`
}

// exchangeCode swaps the authorization code for tokens and fetches the
// account's identity for display.
func (c Config) exchangeCode(client *http.Client, code string) (map[string]interface{}, string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", c.LinkedInRedirectURI)
	form.Set("client_id", c.LinkedInClientID)
	form.Set("client_secret", c.LinkedInClientSecret)
	resp, err := client.PostForm(linkedinTokenURL, form)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, truncate(string(body), 300))
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, "", err
	}
	cfg := map[string]interface{}{
		"accessToken": tok.AccessToken,
		"scope":       tok.Scope,
		"expiresAt":   time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
		"connectedAt": time.Now().UTC().Format(time.RFC3339),
	}
	if tok.RefreshToken != "" {
		cfg["refreshToken"] = tok.RefreshToken
		cfg["refreshExpiresAt"] = time.Now().Add(time.Duration(tok.RefreshTokenExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}

	// Identity (best effort).
	email := ""
	req, _ := http.NewRequest(http.MethodGet, linkedinUserInfo, nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	if r2, err := client.Do(req); err == nil {
		defer r2.Body.Close()
		var info map[string]interface{}
		if json.NewDecoder(r2.Body).Decode(&info) == nil {
			email, _ = info["email"].(string)
			if name, _ := info["name"].(string); name != "" {
				cfg["accountName"] = name
			}
		}
	}
	return cfg, email, nil
}
