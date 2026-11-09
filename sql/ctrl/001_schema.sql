-- ============================================================================
-- RevealDQ_ctrl — platform metadata for Reveal DQ
-- ============================================================================

CREATE DATABASE IF NOT EXISTS RevealDQ_ctrl
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

USE RevealDQ_ctrl;

SET FOREIGN_KEY_CHECKS = 0;

DROP TABLE IF EXISTS dq_job_results;
DROP TABLE IF EXISTS dq_job_runs;
DROP TABLE IF EXISTS dq_pipelines;
DROP TABLE IF EXISTS dq_rule_audit;
DROP TABLE IF EXISTS dq_rules;
DROP TABLE IF EXISTS data_connections;
DROP TABLE IF EXISTS screen_acl;
DROP TABLE IF EXISTS user_workspace_roles;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;

SET FOREIGN_KEY_CHECKS = 1;

CREATE TABLE users (
    user_id         BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    email           VARCHAR(255) NOT NULL UNIQUE,
    password_hash   VARCHAR(255) NOT NULL,
    display_name    VARCHAR(150) NOT NULL,
    is_active       TINYINT(1) NOT NULL DEFAULT 1,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;

CREATE TABLE roles (
    role_id     BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    role_code   VARCHAR(50) NOT NULL UNIQUE,
    role_name   VARCHAR(100) NOT NULL,
    description VARCHAR(255)
) ENGINE=InnoDB;

CREATE TABLE permissions (
    permission_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    permission_code VARCHAR(80) NOT NULL UNIQUE,
    description     VARCHAR(255)
) ENGINE=InnoDB;

CREATE TABLE role_permissions (
    role_id       BIGINT UNSIGNED NOT NULL,
    permission_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (role_id, permission_id),
    CONSTRAINT fk_rp_role FOREIGN KEY (role_id) REFERENCES roles(role_id),
    CONSTRAINT fk_rp_perm FOREIGN KEY (permission_id) REFERENCES permissions(permission_id)
) ENGINE=InnoDB;

CREATE TABLE workspaces (
    workspace_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    workspace_name VARCHAR(150) NOT NULL,
    customer_name  VARCHAR(150) NOT NULL,
    description    VARCHAR(500),
    -- Reality Reveal: product module this project belongs to (see 005_product_modules.sql)
    product_code   VARCHAR(20) NOT NULL DEFAULT 'DQ',
    created_by     BIGINT UNSIGNED NOT NULL,
    created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_ws_creator FOREIGN KEY (created_by) REFERENCES users(user_id)
) ENGINE=InnoDB;

CREATE TABLE user_workspace_roles (
    user_id      BIGINT UNSIGNED NOT NULL,
    workspace_id BIGINT UNSIGNED NOT NULL,
    role_id      BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (user_id, workspace_id),
    CONSTRAINT fk_uwr_user FOREIGN KEY (user_id) REFERENCES users(user_id),
    CONSTRAINT fk_uwr_ws FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id),
    CONSTRAINT fk_uwr_role FOREIGN KEY (role_id) REFERENCES roles(role_id)
) ENGINE=InnoDB;

CREATE TABLE screen_acl (
    screen_acl_id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    role_id       BIGINT UNSIGNED NOT NULL,
    screen_code   VARCHAR(80) NOT NULL,
    can_view      TINYINT(1) NOT NULL DEFAULT 1,
    can_edit      TINYINT(1) NOT NULL DEFAULT 0,
    UNIQUE KEY uk_role_screen (role_id, screen_code),
    CONSTRAINT fk_acl_role FOREIGN KEY (role_id) REFERENCES roles(role_id)
) ENGINE=InnoDB;

CREATE TABLE data_connections (
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
) ENGINE=InnoDB;

CREATE TABLE conversion_audit (
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
) ENGINE=InnoDB;

CREATE TABLE dq_rules (
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
) ENGINE=InnoDB;

-- Traceability / audit trail for rules saved from the DQ Analyser Agent.
-- Captures the analyst prompt, full interaction history, tool/MCP traces, the
-- model + data channel used, and the impact snapshot at save time.
CREATE TABLE dq_rule_audit (
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
) ENGINE=InnoDB;

CREATE TABLE dq_pipelines (
    pipeline_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    workspace_id  BIGINT UNSIGNED NOT NULL,
    pipeline_name VARCHAR(150) NOT NULL,
    proc_name     VARCHAR(128) NOT NULL,
    description   VARCHAR(500),
    is_active     TINYINT(1) NOT NULL DEFAULT 1,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_pipe_ws FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id)
) ENGINE=InnoDB;

CREATE TABLE dq_job_runs (
    run_id         BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    pipeline_id    BIGINT UNSIGNED NOT NULL,
    workspace_id   BIGINT UNSIGNED NOT NULL,
    status         ENUM('PENDING','RUNNING','PASSED','FAILED','ERROR') NOT NULL DEFAULT 'PENDING',
    started_at     DATETIME NULL,
    finished_at    DATETIME NULL,
    rows_checked   BIGINT DEFAULT 0,
    rows_failed    BIGINT DEFAULT 0,
    message        TEXT,
    triggered_by   BIGINT UNSIGNED NULL,
    CONSTRAINT fk_run_pipe FOREIGN KEY (pipeline_id) REFERENCES dq_pipelines(pipeline_id),
    CONSTRAINT fk_run_ws FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id),
    CONSTRAINT fk_run_user FOREIGN KEY (triggered_by) REFERENCES users(user_id)
) ENGINE=InnoDB;

CREATE TABLE dq_job_results (
    result_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    run_id      BIGINT UNSIGNED NOT NULL,
    dimension   VARCHAR(40),
    detail_json JSON,
    CONSTRAINT fk_res_run FOREIGN KEY (run_id) REFERENCES dq_job_runs(run_id)
) ENGINE=InnoDB;
