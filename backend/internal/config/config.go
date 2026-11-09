package config

import (
	"fmt"
	"os"
)

type Config struct {
	APIPort        string
	JWTSecret      string
	DataKey        string // optional DATA_ENCRYPTION_KEY; else derived from JWTSecret
	MySQLDSNCtrl   string
	MySQLDSNSource string
	AgentURL       string
	AdminEmail     string
	AdminPassword  string

	// Reveal VoC
	VocDBName            string
	MySQLDSNVoc          string
	UIURL                string
	LinkedInClientID     string
	LinkedInClientSecret string
	LinkedInRedirectURI  string
	LinkedInAccountEmail string
}

func Load() Config {
	host := env("MYSQL_HOST", "127.0.0.1")
	port := env("MYSQL_PORT", "3306")
	user := env("MYSQL_USER", "root")
	pass := env("MYSQL_PASSWORD", "")
	vocDB := env("MYSQL_VOC_DB", "VOC_DB")
	return Config{
		VocDBName:            vocDB,
		MySQLDSNVoc:          fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true", user, pass, host, port, vocDB),
		UIURL:                env("UI_URL", "http://127.0.0.1:4200"),
		LinkedInClientID:     env("LINKEDIN_CLIENT_ID", ""),
		LinkedInClientSecret: env("LINKEDIN_CLIENT_SECRET", ""),
		LinkedInRedirectURI:  env("LINKEDIN_REDIRECT_URI", "http://127.0.0.1:8080/api/v1/voc/linkedin/oauth/callback"),
		LinkedInAccountEmail: env("LINKEDIN_ACCOUNT_EMAIL", ""),

		APIPort:        env("API_PORT", "8080"),
		JWTSecret:      env("JWT_SECRET", "reality-reveal-dev-secret-change-me"),
		DataKey:        env("DATA_ENCRYPTION_KEY", ""),
		MySQLDSNCtrl:   fmt.Sprintf("%s:%s@tcp(%s:%s)/RevealDQ_ctrl?parseTime=true&multiStatements=true", user, pass, host, port),
		MySQLDSNSource: fmt.Sprintf("%s:%s@tcp(%s:%s)/RevealDQ?parseTime=true&multiStatements=true", user, pass, host, port),
		AgentURL:       env("AGENT_URL", "http://127.0.0.1:8090"),
		AdminEmail:     env("ADMIN_EMAIL", "admin@revealdq.local"),
		AdminPassword:  env("ADMIN_PASSWORD", "Admin@123"),
	}
}

// InsecureSecret is true for empty or well-known placeholder JWT secrets.
func InsecureSecret(secret string) bool {
	switch secret {
	case "", "change-me-in-production", "reality-reveal-dev-secret-change-me":
		return true
	default:
		return false
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
