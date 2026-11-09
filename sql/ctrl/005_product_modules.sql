-- ============================================================================
-- Reality Reveal — multi-product platform
--
-- Introduces the product-module catalog and ties every workspace (project) to
-- exactly one product. Existing workspaces default to Reveal DQ so nothing
-- breaks for projects created before this migration.
--
-- Idempotent: safe to re-run. The Go API also applies the same evolution on
-- startup (backend/internal/db/schema.go), so this file is for manual /
-- CI application of the control-plane schema.
-- ============================================================================
USE RevealDQ_ctrl;

CREATE TABLE IF NOT EXISTS product_modules (
    product_code VARCHAR(20) PRIMARY KEY,
    product_name VARCHAR(100) NOT NULL,
    short_name   VARCHAR(40) NOT NULL,
    tagline      VARCHAR(255) NULL,
    status       ENUM('ACTIVE','PREVIEW','COMING_SOON') NOT NULL DEFAULT 'ACTIVE',
    sort_order   INT NOT NULL DEFAULT 100,
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;

INSERT INTO product_modules (product_code, product_name, short_name, tagline, status, sort_order) VALUES
  ('DQ',  'Reveal DQ',  'DQ',  'The true health of enterprise data — reveal the reality before you migrate.', 'ACTIVE', 10),
  ('VOC', 'Reveal VoC', 'VoC', 'Voice of Customer — reveal what customers are really telling you.', 'PREVIEW', 20)
ON DUPLICATE KEY UPDATE
  product_name = VALUES(product_name),
  short_name   = VALUES(short_name),
  tagline      = VALUES(tagline),
  sort_order   = VALUES(sort_order);

-- workspaces.product_code (additive; MySQL 8 lacks ADD COLUMN IF NOT EXISTS)
SET @has_col := (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'workspaces' AND column_name = 'product_code'
);
SET @ddl := IF(@has_col = 0,
  'ALTER TABLE workspaces ADD COLUMN product_code VARCHAR(20) NOT NULL DEFAULT ''DQ'' AFTER description',
  'SELECT ''workspaces.product_code already present''');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Reveal VoC screen permission + ACL rows
INSERT IGNORE INTO permissions (permission_code, description) VALUES
  ('SCREEN_VOC_HOME', 'Reveal VoC — Voice of Customer overview');

INSERT IGNORE INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id
FROM roles r JOIN permissions p ON p.permission_code = 'SCREEN_VOC_HOME'
WHERE r.role_code IN ('ADMIN','ANALYST','VIEWER');

INSERT IGNORE INTO screen_acl (role_id, screen_code, can_view, can_edit)
SELECT r.role_id, 'SCREEN_VOC_HOME', 1, CASE WHEN r.role_code = 'VIEWER' THEN 0 ELSE 1 END
FROM roles r WHERE r.role_code IN ('ADMIN','ANALYST','VIEWER');
