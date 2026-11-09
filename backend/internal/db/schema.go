package db

import (
	"database/sql"
	"fmt"
)

// EnsureSchema makes the RevealDQ_ctrl control-plane schema self-healing: it
// creates any missing tables (idempotently) and seeds the reference data
// (roles, permissions, screen ACL). This runs on every API startup so that a
// dropped table — e.g. dq_rules — is recreated automatically on the next DB
// connect, instead of the app erroring with "table does not exist".
//
// It is deliberately non-destructive: every statement is CREATE TABLE IF NOT
// EXISTS / INSERT IGNORE, so existing data is never touched.
func EnsureSchema(db *sql.DB) error {
	for _, stmt := range ctrlTables {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	evolveSchema(db)
	seedReferenceData(db)
	return nil
}

// evolveSchema applies additive column migrations to tables that pre-date the
// multi-product platform. Each step is idempotent (checked via information_schema).
func evolveSchema(db *sql.DB) {
	// Reality Reveal: every workspace belongs to exactly one product module.
	addColumnIfMissing(db, "workspaces", "product_code",
		"VARCHAR(20) NOT NULL DEFAULT 'DQ' AFTER description")
}

func addColumnIfMissing(db *sql.DB, table, col, ddl string) {
	var n int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, table, col).Scan(&n)
	if err != nil || n > 0 {
		return
	}
	_, _ = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col, ddl))
}

// ctrlTables is ordered to respect foreign-key dependencies.
var ctrlTables = []string{
	// Product module catalog for the Reality Reveal platform. A workspace is
	// created for exactly one product (Reveal DQ, Reveal VoC, ...).
	`CREATE TABLE IF NOT EXISTS product_modules (
	    product_code VARCHAR(20) PRIMARY KEY,
	    product_name VARCHAR(100) NOT NULL,
	    short_name   VARCHAR(40) NOT NULL,
	    tagline      VARCHAR(255) NULL,
	    status       ENUM('ACTIVE','PREVIEW','COMING_SOON') NOT NULL DEFAULT 'ACTIVE',
	    sort_order   INT NOT NULL DEFAULT 100,
	    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS users (
	    user_id       BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    email         VARCHAR(255) NOT NULL UNIQUE,
	    password_hash VARCHAR(255) NOT NULL,
	    display_name  VARCHAR(150) NOT NULL,
	    is_active     TINYINT(1) NOT NULL DEFAULT 1,
	    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS roles (
	    role_id     BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    role_code   VARCHAR(50) NOT NULL UNIQUE,
	    role_name   VARCHAR(100) NOT NULL,
	    description VARCHAR(255)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS permissions (
	    permission_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    permission_code VARCHAR(80) NOT NULL UNIQUE,
	    description     VARCHAR(255)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS role_permissions (
	    role_id       BIGINT UNSIGNED NOT NULL,
	    permission_id BIGINT UNSIGNED NOT NULL,
	    PRIMARY KEY (role_id, permission_id),
	    CONSTRAINT fk_rp_role FOREIGN KEY (role_id) REFERENCES roles(role_id),
	    CONSTRAINT fk_rp_perm FOREIGN KEY (permission_id) REFERENCES permissions(permission_id)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS workspaces (
	    workspace_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    workspace_name VARCHAR(150) NOT NULL,
	    customer_name  VARCHAR(150) NOT NULL,
	    description    VARCHAR(500),
	    product_code   VARCHAR(20) NOT NULL DEFAULT 'DQ',
	    created_by     BIGINT UNSIGNED NOT NULL,
	    created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    CONSTRAINT fk_ws_creator FOREIGN KEY (created_by) REFERENCES users(user_id)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS user_workspace_roles (
	    user_id      BIGINT UNSIGNED NOT NULL,
	    workspace_id BIGINT UNSIGNED NOT NULL,
	    role_id      BIGINT UNSIGNED NOT NULL,
	    PRIMARY KEY (user_id, workspace_id),
	    CONSTRAINT fk_uwr_user FOREIGN KEY (user_id) REFERENCES users(user_id),
	    CONSTRAINT fk_uwr_ws FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id),
	    CONSTRAINT fk_uwr_role FOREIGN KEY (role_id) REFERENCES roles(role_id)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS screen_acl (
	    screen_acl_id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    role_id       BIGINT UNSIGNED NOT NULL,
	    screen_code   VARCHAR(80) NOT NULL,
	    can_view      TINYINT(1) NOT NULL DEFAULT 1,
	    can_edit      TINYINT(1) NOT NULL DEFAULT 0,
	    UNIQUE KEY uk_role_screen (role_id, screen_code),
	    CONSTRAINT fk_acl_role FOREIGN KEY (role_id) REFERENCES roles(role_id)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS data_connections (
	    connection_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    workspace_id    BIGINT UNSIGNED NOT NULL,
	    connection_name VARCHAR(150) NOT NULL,
	    db_type         VARCHAR(20) NOT NULL DEFAULT 'MYSQL',
	    host            VARCHAR(255) NOT NULL,
	    port            INT NOT NULL DEFAULT 3306,
	    database_name   VARCHAR(128) NOT NULL,
	    username        VARCHAR(128) NOT NULL,
	    password_enc    VARCHAR(2048) NOT NULL DEFAULT '',
	    http_path       VARCHAR(512) NULL,
	    extra_config    JSON NULL,
	    is_active       TINYINT(1) NOT NULL DEFAULT 1,
	    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    CONSTRAINT fk_conn_ws FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS dq_rules (
	    rule_id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    workspace_id         BIGINT UNSIGNED NOT NULL,
	    rule_name            VARCHAR(200) NOT NULL,
	    dimension            ENUM('ACCURACY','COMPLETENESS','CONSISTENCY','TIMELINESS','VALIDITY','UNIQUENESS') NOT NULL,
	    business_definition  TEXT,
	    technical_definition TEXT,
	    impacted_tables      JSON,
	    impacted_columns     JSON,
	    check_sql            TEXT NOT NULL,
	    status               ENUM('DRAFT','APPROVED','ACTIVE','RETIRED') NOT NULL DEFAULT 'DRAFT',
	    proc_name            VARCHAR(128) NULL,
	    created_by           BIGINT UNSIGNED NULL,
	    created_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    CONSTRAINT fk_rule_ws FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id),
	    CONSTRAINT fk_rule_user FOREIGN KEY (created_by) REFERENCES users(user_id)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS dq_rule_audit (
	    audit_id         BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    rule_id          BIGINT UNSIGNED NULL,
	    workspace_id     BIGINT UNSIGNED NOT NULL,
	    rule_name        VARCHAR(200) NOT NULL,
	    dimension        VARCHAR(40) NULL,
	    prompt           TEXT,
	    conversation     JSON,
	    tool_traces      JSON,
	    model_used       VARCHAR(100) NULL,
	    data_source      VARCHAR(20) NULL,
	    impacted_count   BIGINT NULL,
	    total_count      BIGINT NULL,
	    impacted_percent DECIMAL(7,2) NULL,
	    check_sql        TEXT,
	    status           VARCHAR(20) NULL,
	    created_by       BIGINT UNSIGNED NULL,
	    created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    KEY idx_audit_ws (workspace_id),
	    KEY idx_audit_rule (rule_id),
	    CONSTRAINT fk_audit_ws FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS conversion_audit (
	    conversion_id  BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    workspace_id   BIGINT UNSIGNED NOT NULL,
	    file_name      VARCHAR(255) NOT NULL,
	    source_type    VARCHAR(40) NULL,
	    source_label   VARCHAR(120) NULL,
	    target         VARCHAR(40) NULL,
	    model_used     VARCHAR(100) NULL,
	    engine         VARCHAR(20) NULL,
	    plan_md        MEDIUMTEXT,
	    source_code    MEDIUMTEXT,
	    converted_code MEDIUMTEXT,
	    language       VARCHAR(20) NULL,
	    validation     JSON,
	    status         VARCHAR(30) NULL,
	    git_path       VARCHAR(512) NULL,
	    notebook_path  VARCHAR(512) NULL,
	    job_id         VARCHAR(64) NULL,
	    run_id         VARCHAR(64) NULL,
	    created_by     BIGINT UNSIGNED NULL,
	    created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    KEY idx_conv_ws (workspace_id)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS dq_pipelines (
	    pipeline_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    workspace_id  BIGINT UNSIGNED NOT NULL,
	    pipeline_name VARCHAR(150) NOT NULL,
	    proc_name     VARCHAR(128) NOT NULL,
	    description   VARCHAR(500),
	    is_active     TINYINT(1) NOT NULL DEFAULT 1,
	    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    CONSTRAINT fk_pipe_ws FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS dq_job_runs (
	    run_id       BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    pipeline_id  BIGINT UNSIGNED NOT NULL,
	    workspace_id BIGINT UNSIGNED NOT NULL,
	    status       ENUM('PENDING','RUNNING','PASSED','FAILED','ERROR') NOT NULL DEFAULT 'PENDING',
	    started_at   DATETIME NULL,
	    finished_at  DATETIME NULL,
	    rows_checked BIGINT DEFAULT 0,
	    rows_failed  BIGINT DEFAULT 0,
	    message      TEXT,
	    triggered_by BIGINT UNSIGNED NULL,
	    CONSTRAINT fk_run_pipe FOREIGN KEY (pipeline_id) REFERENCES dq_pipelines(pipeline_id),
	    CONSTRAINT fk_run_ws FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id),
	    CONSTRAINT fk_run_user FOREIGN KEY (triggered_by) REFERENCES users(user_id)
	) ENGINE=InnoDB`,

	`CREATE TABLE IF NOT EXISTS dq_job_results (
	    result_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
	    run_id      BIGINT UNSIGNED NOT NULL,
	    dimension   VARCHAR(40),
	    detail_json JSON,
	    CONSTRAINT fk_res_run FOREIGN KEY (run_id) REFERENCES dq_job_runs(run_id)
	) ENGINE=InnoDB`,
}

// seedReferenceData inserts the baseline roles, permissions and screen ACL if
// they are missing. All inserts are idempotent (unique keys + INSERT IGNORE).
func seedReferenceData(db *sql.DB) {
	exec := func(q string) {
		if _, err := db.Exec(q); err != nil {
			// Reference seeding is best-effort; log-less to avoid noise on re-runs.
			_ = err
		}
	}

	// Product module catalog. Statuses: ACTIVE (fully built), PREVIEW (shell +
	// roadmap, projects can be created), COMING_SOON (listed, not creatable).
	exec(`INSERT INTO product_modules (product_code, product_name, short_name, tagline, status, sort_order) VALUES
		('DQ',  'Reveal DQ',  'DQ',  'The true health of enterprise data — reveal the reality before you migrate.', 'ACTIVE', 10),
		('VOC', 'Reveal VoC', 'VoC', 'Voice of Customer — reveal what customers are really telling you.', 'PREVIEW', 20)
		ON DUPLICATE KEY UPDATE product_name=VALUES(product_name), short_name=VALUES(short_name),
		  tagline=VALUES(tagline), sort_order=VALUES(sort_order)`)

	exec(`INSERT IGNORE INTO roles (role_code, role_name, description) VALUES
		('ADMIN','Admin','Full access to all screens and actions'),
		('ANALYST','Analyst','Can analyse data and manage DQ rules and pipelines'),
		('VIEWER','Viewer','Read-only access to dashboards and rules')`)

	exec(`INSERT IGNORE INTO permissions (permission_code, description) VALUES
		('SCREEN_HOME','Access home / workspaces'),
		('SCREEN_ANALYSER','DQ Analyser Agent'),
		('SCREEN_RULES','DQ Rules Engine'),
		('SCREEN_ETL','ETL Analyser Agent'),
		('SCREEN_INFORMATICA','ETL Code Conversion Agent'),
		('SCREEN_DASHBOARD','Operations dashboard'),
		('SCREEN_PIPELINES','DQ Pipelines'),
		('SCREEN_CONFIG','Data source config'),
		('SCREEN_SETTINGS','Security and access control'),
		('SCREEN_VOC_HOME','Reveal VoC — Voice of Customer overview')`)

	// Admin: every permission.
	exec(`INSERT IGNORE INTO role_permissions (role_id, permission_id)
		SELECT r.role_id, p.permission_id FROM roles r CROSS JOIN permissions p WHERE r.role_code='ADMIN'`)

	// Analyst: working screens (no settings).
	exec(`INSERT IGNORE INTO role_permissions (role_id, permission_id)
		SELECT r.role_id, p.permission_id FROM roles r JOIN permissions p
		  ON p.permission_code IN ('SCREEN_HOME','SCREEN_ANALYSER','SCREEN_RULES','SCREEN_DASHBOARD',
		     'SCREEN_PIPELINES','SCREEN_CONFIG','SCREEN_ETL','SCREEN_INFORMATICA','SCREEN_VOC_HOME')
		WHERE r.role_code='ANALYST'`)

	// Viewer: read-only screens.
	exec(`INSERT IGNORE INTO role_permissions (role_id, permission_id)
		SELECT r.role_id, p.permission_id FROM roles r JOIN permissions p
		  ON p.permission_code IN ('SCREEN_HOME','SCREEN_DASHBOARD','SCREEN_RULES','SCREEN_VOC_HOME')
		WHERE r.role_code='VIEWER'`)

	exec(`INSERT IGNORE INTO screen_acl (role_id, screen_code, can_view, can_edit)
		SELECT r.role_id, p.permission_code, 1,
		       CASE WHEN r.role_code='VIEWER' THEN 0
		            WHEN r.role_code='ANALYST' AND p.permission_code='SCREEN_SETTINGS' THEN 0
		            ELSE 1 END
		FROM roles r CROSS JOIN permissions p
		WHERE (r.role_code='ADMIN')
		   OR (r.role_code='ANALYST' AND p.permission_code IN (
		        'SCREEN_HOME','SCREEN_ANALYSER','SCREEN_RULES','SCREEN_DASHBOARD',
		        'SCREEN_PIPELINES','SCREEN_CONFIG','SCREEN_ETL','SCREEN_INFORMATICA','SCREEN_VOC_HOME'))
		   OR (r.role_code='VIEWER' AND p.permission_code IN (
		        'SCREEN_HOME','SCREEN_DASHBOARD','SCREEN_RULES','SCREEN_VOC_HOME'))`)
}
