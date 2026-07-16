package config

import (
	"fmt"
	"os"
)

type Config struct {
	APIPort        string
	JWTSecret      string
	MySQLDSNCtrl   string
	MySQLDSNSource string
	AgentURL       string
	AdminEmail     string
	AdminPassword  string
}

func Load() Config {
	host := env("MYSQL_HOST", "127.0.0.1")
	port := env("MYSQL_PORT", "3306")
	user := env("MYSQL_USER", "root")
	pass := env("MYSQL_PASSWORD", "")
	return Config{
		APIPort:        env("API_PORT", "8080"),
		JWTSecret:      env("JWT_SECRET", "reveal-dq-dev-secret-change-me"),
		MySQLDSNCtrl:   fmt.Sprintf("%s:%s@tcp(%s:%s)/RevealDQ_ctrl?parseTime=true&multiStatements=true", user, pass, host, port),
		MySQLDSNSource: fmt.Sprintf("%s:%s@tcp(%s:%s)/RevealDQ?parseTime=true&multiStatements=true", user, pass, host, port),
		AgentURL:       env("AGENT_URL", "http://127.0.0.1:8090"),
		AdminEmail:     env("ADMIN_EMAIL", "admin@revealdq.local"),
		AdminPassword:  env("ADMIN_PASSWORD", "Admin@123"),
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
