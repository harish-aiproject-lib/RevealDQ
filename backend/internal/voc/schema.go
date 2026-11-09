// Package voc implements the Reveal VoC (Voice of Customer) module.
//
// Responsibilities split deliberately:
//   - Extraction from LinkedIn is deterministic AUTOMATION (this package):
//     OAuth-backed official API, a pluggable JSON data-provider adapter, a
//     file/JSON import path and a sample generator for local development.
//   - Analytics (sentiment, category, themes, actions) is AGENTIC and lives in
//     the Python agent (agents/voc_analysis.py); this package only orchestrates
//     the calls and persists the results.
//
// All raw and derived data is stored in its own MySQL database (default
// VOC_DB), created on API start if missing.
package voc

import (
	"database/sql"
	"fmt"
	"regexp"
)

var identRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// EnsureDatabase creates the VoC database if it does not exist, using an
// already-open connection to the control-plane server.
func EnsureDatabase(ctrl *sql.DB, name string) error {
	if !identRe.MatchString(name) {
		return fmt.Errorf("invalid VOC database name %q", name)
	}
	_, err := ctrl.Exec(fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", name))
	return err
}

// EnsureSchema creates/evolves every VoC table. Idempotent.
func EnsureSchema(db *sql.DB) error {
	for _, stmt := range tables {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("voc schema: %w", err)
		}
	}
	return nil
}

var tables = []string{
	// Companies the workspace wants to listen to on LinkedIn.
	`CREATE TABLE IF NOT EXISTS voc_companies (
	    company_id      BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    workspace_id    BIGINT UNSIGNED NOT NULL,
	    company_name    VARCHAR(200) NOT NULL,
	    linkedin_handle VARCHAR(200) NOT NULL,
	    linkedin_urn    VARCHAR(120) NULL,
	    hashtags        JSON NULL,
	    is_active       TINYINT(1) NOT NULL DEFAULT 1,
	    last_sync_at    DATETIME NULL,
	    created_by      BIGINT UNSIGNED NULL,
	    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    UNIQUE KEY uk_ws_handle (workspace_id, linkedin_handle),
	    KEY idx_company_ws (workspace_id)
	) ENGINE=InnoDB`,

	// How a workspace reaches LinkedIn. One row per provider.
	//   OFFICIAL_API  – LinkedIn Community Management API via OAuth (config holds tokens)
	//   GENERIC_JSON  – any HTTP endpoint that returns normalised items (hashtag search etc.)
	//   IMPORT        – manual JSON/CSV upload through the UI
	//   SAMPLE        – synthetic generator for local development
	`CREATE TABLE IF NOT EXISTS voc_sources (
	    source_id     BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    workspace_id  BIGINT UNSIGNED NOT NULL,
	    channel       VARCHAR(30) NOT NULL DEFAULT 'LINKEDIN',
	    provider      VARCHAR(30) NOT NULL,
	    label         VARCHAR(150) NOT NULL,
	    account_email VARCHAR(255) NULL,
	    config        JSON NULL,
	    status        VARCHAR(30) NOT NULL DEFAULT 'CONFIGURED',
	    is_active     TINYINT(1) NOT NULL DEFAULT 1,
	    last_sync_at  DATETIME NULL,
	    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    KEY idx_source_ws (workspace_id)
	) ENGINE=InnoDB`,

	// Raw structured signals exactly as collected (normalised columns + raw_json).
	`CREATE TABLE IF NOT EXISTS voc_posts (
	    post_id            BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    workspace_id       BIGINT UNSIGNED NOT NULL,
	    company_id         BIGINT UNSIGNED NOT NULL,
	    source_id          BIGINT UNSIGNED NULL,
	    sync_run_id        BIGINT UNSIGNED NULL,
	    channel            VARCHAR(30) NOT NULL DEFAULT 'LINKEDIN',
	    external_id        VARCHAR(255) NOT NULL,
	    kind               VARCHAR(20) NOT NULL DEFAULT 'POST',
	    parent_external_id VARCHAR(255) NULL,
	    author_name        VARCHAR(200) NULL,
	    author_handle      VARCHAR(200) NULL,
	    author_headline    VARCHAR(300) NULL,
	    author_type        VARCHAR(20) NULL,
	    content            MEDIUMTEXT,
	    url                VARCHAR(600) NULL,
	    hashtags           JSON NULL,
	    mentions           JSON NULL,
	    like_count         INT NOT NULL DEFAULT 0,
	    comment_count      INT NOT NULL DEFAULT 0,
	    share_count        INT NOT NULL DEFAULT 0,
	    published_at       DATETIME NULL,
	    collected_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    raw_json           JSON NULL,
	    UNIQUE KEY uk_ws_external (workspace_id, external_id),
	    KEY idx_post_company (company_id, published_at),
	    KEY idx_post_ws_pub (workspace_id, published_at)
	) ENGINE=InnoDB`,

	// Agentic analysis result per post (one row, replaced on re-analysis).
	`CREATE TABLE IF NOT EXISTS voc_post_analysis (
	    post_id          BIGINT UNSIGNED PRIMARY KEY,
	    workspace_id     BIGINT UNSIGNED NOT NULL,
	    sentiment        ENUM('POSITIVE','NEGATIVE','NEUTRAL','BROADCAST') NOT NULL,
	    sentiment_score  DECIMAL(4,3) NULL,
	    category         VARCHAR(40) NULL,
	    themes           JSON NULL,
	    summary          VARCHAR(600) NULL,
	    urgency          ENUM('LOW','MEDIUM','HIGH') NOT NULL DEFAULT 'LOW',
	    suggested_action VARCHAR(400) NULL,
	    engine           VARCHAR(20) NULL,
	    model_used       VARCHAR(100) NULL,
	    analysed_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
	    KEY idx_analysis_ws (workspace_id, sentiment),
	    KEY idx_analysis_cat (workspace_id, category)
	) ENGINE=InnoDB`,

	// Actions created by the agent (or manually) from the analysed signals.
	`CREATE TABLE IF NOT EXISTS voc_actions (
	    action_id    BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    workspace_id BIGINT UNSIGNED NOT NULL,
	    company_id   BIGINT UNSIGNED NULL,
	    title        VARCHAR(300) NOT NULL,
	    description  TEXT,
	    category     VARCHAR(40) NULL,
	    priority     ENUM('LOW','MEDIUM','HIGH') NOT NULL DEFAULT 'MEDIUM',
	    status       ENUM('OPEN','IN_PROGRESS','DONE','DISMISSED') NOT NULL DEFAULT 'OPEN',
	    source       VARCHAR(20) NOT NULL DEFAULT 'AGENT',
	    evidence     JSON NULL,
	    signal_count INT NOT NULL DEFAULT 0,
	    dedupe_key   VARCHAR(200) NULL,
	    owner        VARCHAR(150) NULL,
	    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
	    KEY idx_action_ws (workspace_id, status),
	    KEY idx_action_dedupe (workspace_id, dedupe_key)
	) ENGINE=InnoDB`,

	// Every extraction + analysis run (manual or scheduled).
	`CREATE TABLE IF NOT EXISTS voc_sync_runs (
	    run_id         BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    workspace_id   BIGINT UNSIGNED NOT NULL,
	    company_id     BIGINT UNSIGNED NULL,
	    source_id      BIGINT UNSIGNED NULL,
	    provider       VARCHAR(30) NULL,
	    trigger_type   VARCHAR(20) NOT NULL DEFAULT 'MANUAL',
	    status         ENUM('RUNNING','SUCCESS','PARTIAL','FAILED') NOT NULL DEFAULT 'RUNNING',
	    started_at     DATETIME NULL,
	    finished_at    DATETIME NULL,
	    items_fetched  INT NOT NULL DEFAULT 0,
	    items_new      INT NOT NULL DEFAULT 0,
	    items_analysed INT NOT NULL DEFAULT 0,
	    actions_created INT NOT NULL DEFAULT 0,
	    message        TEXT,
	    triggered_by   BIGINT UNSIGNED NULL,
	    KEY idx_run_ws (workspace_id, run_id)
	) ENGINE=InnoDB`,

	// Periodic refresh schedule (one row per workspace).
	`CREATE TABLE IF NOT EXISTS voc_schedules (
	    workspace_id     BIGINT UNSIGNED PRIMARY KEY,
	    enabled          TINYINT(1) NOT NULL DEFAULT 0,
	    cadence_minutes  INT NOT NULL DEFAULT 360,
	    next_run_at      DATETIME NULL,
	    last_run_at      DATETIME NULL,
	    last_status      VARCHAR(20) NULL,
	    updated_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB`,
}
