package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func EnsureAdmin(db *sql.DB, email, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	var id int64
	err = db.QueryRow(`SELECT user_id FROM users WHERE email = ?`, email).Scan(&id)
	if err == sql.ErrNoRows {
		res, err := db.Exec(`INSERT INTO users (email, password_hash, display_name) VALUES (?, ?, ?)`,
			email, string(hash), "Reveal DQ Admin")
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
	} else if err != nil {
		return fmt.Errorf("lookup admin: %w", err)
	} else {
		_, _ = db.Exec(`UPDATE users SET password_hash=? WHERE user_id=?`, string(hash), id)
	}
	var roleID int64
	_ = db.QueryRow(`SELECT role_id FROM roles WHERE role_code='ADMIN'`).Scan(&roleID)
	var wsID int64
	_ = db.QueryRow(`SELECT workspace_id FROM workspaces LIMIT 1`).Scan(&wsID)
	if roleID > 0 && wsID > 0 {
		_, _ = db.Exec(`INSERT IGNORE INTO user_workspace_roles (user_id, workspace_id, role_id) VALUES (?,?,?)`,
			id, wsID, roleID)
	}
	return nil
}
