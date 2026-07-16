-- ============================================================================
-- RevealDQ_ctrl seed — roles, permissions, ACL, admin user
-- Default password for admin@revealdq.local is: Admin@123
-- bcrypt hash generated for: Admin@123
-- ============================================================================

USE RevealDQ_ctrl;

INSERT INTO roles (role_code, role_name, description) VALUES
('ADMIN', 'Admin', 'Full access to all screens and actions'),
('ANALYST', 'Analyst', 'Can analyse data and manage DQ rules and pipelines'),
('VIEWER', 'Viewer', 'Read-only access to dashboards and rules');

INSERT INTO permissions (permission_code, description) VALUES
('SCREEN_HOME', 'Access home / workspaces'),
('SCREEN_ANALYSER', 'DQ Analyser Agent'),
('SCREEN_RULES', 'DQ Rules Engine'),
('SCREEN_ETL', 'ETL Analyser Agent'),
('SCREEN_INFORMATICA', 'Informatica DQ Conversion Agent'),
('SCREEN_DASHBOARD', 'Operations dashboard'),
('SCREEN_PIPELINES', 'DQ Pipelines'),
('SCREEN_CONFIG', 'Data source config'),
('SCREEN_SETTINGS', 'Security and access control');

-- Admin: all screens view+edit
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id FROM roles r CROSS JOIN permissions p WHERE r.role_code = 'ADMIN';

-- Analyst: analyser, rules, dashboard, pipelines, config (no settings edit)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id FROM roles r
JOIN permissions p ON p.permission_code IN (
  'SCREEN_HOME','SCREEN_ANALYSER','SCREEN_RULES','SCREEN_DASHBOARD',
  'SCREEN_PIPELINES','SCREEN_CONFIG','SCREEN_ETL','SCREEN_INFORMATICA'
)
WHERE r.role_code = 'ANALYST';

-- Viewer: home, dashboard, rules view
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id FROM roles r
JOIN permissions p ON p.permission_code IN (
  'SCREEN_HOME','SCREEN_DASHBOARD','SCREEN_RULES'
)
WHERE r.role_code = 'VIEWER';

INSERT INTO screen_acl (role_id, screen_code, can_view, can_edit)
SELECT r.role_id, p.permission_code, 1,
  CASE WHEN r.role_code = 'VIEWER' THEN 0
       WHEN r.role_code = 'ANALYST' AND p.permission_code = 'SCREEN_SETTINGS' THEN 0
       ELSE 1 END
FROM roles r
CROSS JOIN permissions p
WHERE (r.role_code = 'ADMIN')
   OR (r.role_code = 'ANALYST' AND p.permission_code IN (
        'SCREEN_HOME','SCREEN_ANALYSER','SCREEN_RULES','SCREEN_DASHBOARD',
        'SCREEN_PIPELINES','SCREEN_CONFIG','SCREEN_ETL','SCREEN_INFORMATICA'))
   OR (r.role_code = 'VIEWER' AND p.permission_code IN (
        'SCREEN_HOME','SCREEN_DASHBOARD','SCREEN_RULES'));

-- Admin user is created by the Go API on startup (EnsureAdmin) with ADMIN_PASSWORD.
-- Demo workspace / connection / pipelines are created after first login via UI,
-- or by running sql/ctrl/003_demo_workspace.sql after the admin user exists.