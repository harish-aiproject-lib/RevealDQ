-- Run AFTER Go API has created admin@revealdq.local (or register a user).
USE RevealDQ_ctrl;

INSERT INTO workspaces (workspace_name, customer_name, description, created_by)
SELECT 'Telco RA Demo', 'Acme Telecom', 'Sample revenue assurance / DQ workspace over RevealDQ', user_id
FROM users WHERE email = 'admin@revealdq.local'
AND NOT EXISTS (SELECT 1 FROM workspaces WHERE workspace_name = 'Telco RA Demo');

INSERT IGNORE INTO user_workspace_roles (user_id, workspace_id, role_id)
SELECT u.user_id, w.workspace_id, r.role_id
FROM users u
CROSS JOIN workspaces w
CROSS JOIN roles r
WHERE u.email = 'admin@revealdq.local'
  AND w.workspace_name = 'Telco RA Demo'
  AND r.role_code = 'ADMIN';

INSERT INTO data_connections (workspace_id, connection_name, db_type, host, port, database_name, username, password_enc)
SELECT w.workspace_id, 'Local RevealDQ MySQL', 'MYSQL', '127.0.0.1', 3306, 'RevealDQ', 'root',
       IFNULL(@mysql_password, 'CHANGE_ME')
FROM workspaces w
WHERE w.workspace_name = 'Telco RA Demo'
  AND NOT EXISTS (
    SELECT 1 FROM data_connections c WHERE c.workspace_id = w.workspace_id AND c.connection_name = 'Local RevealDQ MySQL'
  );

INSERT INTO dq_pipelines (workspace_id, pipeline_name, proc_name, description)
SELECT w.workspace_id, v.pipeline_name, v.proc_name, v.description
FROM workspaces w
JOIN (
  SELECT 'CDR Unrated Completeness' AS pipeline_name, 'sp_dq_cdr_unrated' AS proc_name,
         'Detects usage_cdr rows with no corresponding rated_usage' AS description
  UNION ALL
  SELECT 'Invoice Balance Consistency', 'sp_dq_invoice_balance',
         'Detects invoices whose total_amount does not match line items'
  UNION ALL
  SELECT 'MSISDN Validity', 'sp_dq_msisdn_validity',
         'Detects service_identifier values that fail MSISDN format checks'
) v
WHERE w.workspace_name = 'Telco RA Demo'
  AND NOT EXISTS (
    SELECT 1 FROM dq_pipelines p WHERE p.workspace_id = w.workspace_id AND p.proc_name = v.proc_name
  );
