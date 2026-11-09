-- ============================================================================
-- Migration: DQ Analyser rule-save audit trail (idempotent).
-- Safe to run on an already-provisioned RevealDQ_ctrl database.
-- The Go API also creates this table on demand, so applying this is optional.
-- ============================================================================

USE RevealDQ_ctrl;

CREATE TABLE IF NOT EXISTS dq_rule_audit (
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
    KEY idx_audit_rule (rule_id)
) ENGINE=InnoDB;
