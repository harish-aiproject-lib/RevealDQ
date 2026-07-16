-- ============================================================================
-- RevealDQ seed data (~50 usage_cdr) with intentional DQ defects
-- ============================================================================

USE RevealDQ;

SET FOREIGN_KEY_CHECKS = 0;
TRUNCATE TABLE payment;
TRUNCATE TABLE invoice_line_item;
TRUNCATE TABLE invoice;
TRUNCATE TABLE rated_usage;
TRUNCATE TABLE usage_cdr;
TRUNCATE TABLE service_subscription;
TRUNCATE TABLE network_element;
TRUNCATE TABLE product_catalog;
TRUNCATE TABLE account;
TRUNCATE TABLE customer;
SET FOREIGN_KEY_CHECKS = 1;

-- ---------------------------------------------------------------------------
-- Customers (12)
-- ---------------------------------------------------------------------------
INSERT INTO customer (customer_id, customer_name, customer_type, segment, registration_date, kyc_status, customer_status) VALUES
(1,  'Olivia Chen',          'INDIVIDUAL', 'Consumer',  '2023-01-15', 'VERIFIED', 'ACTIVE'),
(2,  'Marcus Webb',          'INDIVIDUAL', 'Consumer',  '2023-02-20', 'VERIFIED', 'ACTIVE'),
(3,  'Acme Mining Ltd',      'ENTERPRISE', 'Corporate', '2022-06-01', 'VERIFIED', 'ACTIVE'),
(4,  'Northern Rail Co',     'ENTERPRISE', 'Corporate', '2022-08-12', 'VERIFIED', 'ACTIVE'),
(5,  'Priya Nair',           'INDIVIDUAL', 'SME',       '2023-03-10', 'PENDING',  'ACTIVE'),
(6,  'James Okonkwo',        'INDIVIDUAL', 'Consumer',  '2023-04-05', 'VERIFIED', 'SUSPENDED'),
(7,  'Blue Ocean Retail',    'ENTERPRISE', 'SME',       '2021-11-30', 'VERIFIED', 'ACTIVE'),
(8,  'Sofia Alvarez',        'INDIVIDUAL', 'Consumer',  '2023-05-18', 'REJECTED', 'CHURNED'),
(9,  'Pacific Logistics',    'ENTERPRISE', 'Corporate', '2020-09-01', 'VERIFIED', 'ACTIVE'),
(10, 'Hannah Brooks',        'INDIVIDUAL', 'Consumer',  '2023-06-22', 'VERIFIED', 'ACTIVE'),
(11, 'Orbit Media Pty',      'ENTERPRISE', 'SME',       '2022-01-14', 'VERIFIED', 'ACTIVE'),
(12, 'Daniel Kim',           'INDIVIDUAL', 'Consumer',  '2023-07-01', 'PENDING',  'ACTIVE');

-- ---------------------------------------------------------------------------
-- Accounts (15) — includes validity defect: billing_cycle_day=35
-- ---------------------------------------------------------------------------
INSERT INTO account (account_id, customer_id, account_type, billing_cycle_day, currency_code, credit_limit, account_status) VALUES
(1,  1,  'POSTPAID', 1,  'AUD', 500.00,   'ACTIVE'),
(2,  2,  'PREPAID',  15, 'AUD', 0.00,     'ACTIVE'),
(3,  3,  'POSTPAID', 28, 'AUD', 50000.00, 'ACTIVE'),
(4,  3,  'POSTPAID', 28, 'AUD', 10000.00, 'ACTIVE'),
(5,  4,  'POSTPAID', 5,  'AUD', 25000.00, 'ACTIVE'),
(6,  5,  'POSTPAID', 10, 'AUD', 1000.00,  'ACTIVE'),
(7,  6,  'PREPAID',  1,  'AUD', 0.00,     'SUSPENDED'),
(8,  7,  'POSTPAID', 20, 'AUD', 8000.00,  'ACTIVE'),
(9,  8,  'POSTPAID', 12, 'AUD', 200.00,   'CLOSED'),
(10, 9,  'POSTPAID', 1,  'AUD', 40000.00, 'ACTIVE'),
(11, 9,  'POSTPAID', 1,  'USD', 15000.00, 'ACTIVE'), -- currency inconsistency vs peers
(12, 10, 'PREPAID',  8,  'AUD', 0.00,     'ACTIVE'),
(13, 11, 'POSTPAID', 35, 'AUD', 5000.00,  'ACTIVE'), -- VALIDITY: day 35 invalid
(14, 12, 'POSTPAID', 14, 'AUD', 750.00,   'ACTIVE'),
(15, 4,  'PREPAID',  22, 'AUD', 0.00,     'ACTIVE');

-- ---------------------------------------------------------------------------
-- Products (8)
-- ---------------------------------------------------------------------------
INSERT INTO product_catalog (product_id, product_name, product_type, rate_plan_code, base_price, uom, effective_start_date, effective_end_date) VALUES
(1, 'Voice Anytime',     'VOICE',  'VOICE_ANY',  0.25, 'MIN',   '2022-01-01', NULL),
(2, 'Data Unlimited Lite','DATA',  'DATA_UL_L', 39.99,'MONTH', '2022-01-01', NULL),
(3, 'SMS Pack 500',      'SMS',   'SMS_500',    0.10, 'EVENT', '2022-01-01', NULL),
(4, 'Business Bundle+',  'BUNDLE','BIZ_PLUS',  89.00,'MONTH', '2022-06-01', NULL),
(5, 'Roam Voice Zone A', 'VOICE',  'ROAM_VA',   1.50, 'MIN',   '2022-01-01', NULL),
(6, 'Data Turbo 50GB',   'DATA',  'DATA_50',   29.99,'MONTH', '2023-01-01', NULL),
(7, 'SMS PayG',          'SMS',   'SMS_PAYG',   0.22, 'EVENT', '2022-01-01', NULL),
(8, 'Legacy Voice',      'VOICE',  'LEGACY_V',  0.40, 'MIN',   '2019-01-01', '2023-12-31');

-- ---------------------------------------------------------------------------
-- Network elements (5)
-- ---------------------------------------------------------------------------
INSERT INTO network_element (network_element_id, element_name, element_type, source_system_code, region) VALUES
(1, 'MSC-SYD-01',     'MSC',       'MED_MSC_SYD',  'NSW'),
(2, 'SGSN-MEL-02',    'SGSN',      'MED_SGSN_MEL', 'VIC'),
(3, 'SOFTSW-BNE-01',  'SOFTSWITCH','MED_SS_BNE',   'QLD'),
(4, 'GGSN-SYD-03',    'GGSN',      'MED_GGSN_SYD', 'NSW'),
(5, 'IN-PER-01',      'IN',        'MED_IN_PER',   'WA');

-- ---------------------------------------------------------------------------
-- Subscriptions (20) — includes invalid MSISDN for validity checks
-- ---------------------------------------------------------------------------
INSERT INTO service_subscription (subscription_id, account_id, product_id, service_identifier, activation_date, deactivation_date, subscription_status) VALUES
(1,  1,  1, '61400100001', '2023-01-20', NULL, 'ACTIVE'),
(2,  1,  2, '61400100001', '2023-01-20', NULL, 'ACTIVE'),
(3,  2,  3, '61400100002', '2023-02-25', NULL, 'ACTIVE'),
(4,  3,  4, '61400110001', '2022-06-15', NULL, 'ACTIVE'),
(5,  3,  6, '61400110002', '2022-06-15', NULL, 'ACTIVE'),
(6,  4,  4, '61400110003', '2022-07-01', NULL, 'ACTIVE'),
(7,  5,  4, '61400220001', '2022-08-20', NULL, 'ACTIVE'),
(8,  5,  1, '61400220002', '2022-08-20', NULL, 'ACTIVE'),
(9,  6,  2, '61400330001', '2023-03-15', NULL, 'ACTIVE'),
(10, 7,  1, '61400440001', '2023-04-10', '2023-09-01', 'SUSPENDED'),
(11, 8,  4, '61400550001', '2021-12-01', NULL, 'ACTIVE'),
(12, 8,  7, '61400550002', '2021-12-01', NULL, 'ACTIVE'),
(13, 9,  1, 'BAD-MSISDN',  '2023-05-20', '2023-08-01', 'TERMINATED'), -- VALIDITY
(14, 10, 4, '61400660001', '2020-09-15', NULL, 'ACTIVE'),
(15, 10, 5, '61400660002', '2020-09-15', NULL, 'ACTIVE'),
(16, 11, 6, '61400660003', '2021-01-10', NULL, 'ACTIVE'),
(17, 12, 3, '61400770001', '2023-06-25', NULL, 'ACTIVE'),
(18, 13, 2, '61400880001', '2022-02-01', NULL, 'ACTIVE'),
(19, 14, 1, '61400990001', '2023-07-05', NULL, 'ACTIVE'),
(20, 15, 7, '61400220099', '2023-01-01', NULL, 'ACTIVE');

-- ---------------------------------------------------------------------------
-- Usage CDR (~50) with defects: duplicates, unrated, null dest, negative volume,
-- mediation before usage (timeliness)
-- ---------------------------------------------------------------------------
INSERT INTO usage_cdr (cdr_id, subscription_id, network_element_id, usage_type, usage_start_timestamp, duration_seconds, volume_kb, origin_number, destination_number, roaming_flag, mediation_timestamp, cdr_status) VALUES
(1,  1, 1, 'VOICE', '2024-06-01 09:00:00', 120, 0,     '61400100001', '61411110001', 0, '2024-06-01 09:05:00', 'PROCESSED'),
(2,  1, 1, 'VOICE', '2024-06-01 10:00:00', 60,  0,     '61400100001', '61411110002', 0, '2024-06-01 10:02:00', 'PROCESSED'),
(3,  2, 4, 'DATA',  '2024-06-01 11:00:00', 0,   51200, '61400100001', NULL,          0, '2024-06-01 11:10:00', 'PROCESSED'),
(4,  3, 1, 'SMS',   '2024-06-01 12:00:00', 0,   0,     '61400100002', '61411110003', 0, '2024-06-01 12:01:00', 'PROCESSED'),
(5,  4, 3, 'VOICE', '2024-06-02 08:00:00', 300, 0,     '61400110001', '61422220001', 0, '2024-06-02 08:06:00', 'PROCESSED'),
(6,  4, 3, 'VOICE', '2024-06-02 09:00:00', 45,  0,     '61400110001', NULL,          0, '2024-06-02 09:01:00', 'RAW'), -- COMPLETENESS: null dest voice
(7,  5, 4, 'DATA',  '2024-06-02 10:00:00', 0,   102400,'61400110002', NULL,          0, '2024-06-02 10:15:00', 'PROCESSED'),
(8,  6, 3, 'VOICE', '2024-06-02 11:00:00', 90,  0,     '61400110003', '61422220002', 0, '2024-06-02 11:03:00', 'PROCESSED'),
(9,  7, 2, 'DATA',  '2024-06-03 07:00:00', 0,   8192,  '61400220001', NULL,          0, '2024-06-03 07:05:00', 'PROCESSED'),
(10, 8, 1, 'VOICE', '2024-06-03 08:00:00', 180, 0,     '61400220002', '61433330001', 0, '2024-06-03 08:04:00', 'PROCESSED'),
(11, 9, 4, 'DATA',  '2024-06-03 09:00:00', 0,   20480, '61400330001', NULL,          0, '2024-06-03 09:20:00', 'PROCESSED'),
(12, 10,1, 'VOICE', '2024-06-03 10:00:00', 30,  0,     '61400440001', '61433330002', 0, '2024-06-03 10:01:00', 'SUSPECT'),
(13, 11,3, 'VOICE', '2024-06-04 08:00:00', 240, 0,     '61400550001', '61444440001', 0, '2024-06-04 08:05:00', 'PROCESSED'),
(14, 12,1, 'SMS',   '2024-06-04 09:00:00', 0,   0,     '61400550002', '61444440002', 0, '2024-06-04 09:01:00', 'PROCESSED'),
(15, 13,1, 'VOICE', '2024-06-04 10:00:00', 15,  0,     'BAD-MSISDN',  '61455550001', 0, '2024-06-04 10:02:00', 'RAW'),
(16, 14,3, 'VOICE', '2024-06-05 07:00:00', 600, 0,     '61400660001', '61466660001', 0, '2024-06-05 07:12:00', 'PROCESSED'),
(17, 15,1, 'VOICE', '2024-06-05 08:00:00', 90,  0,     '61400660002', '61466660002', 1, '2024-06-05 08:03:00', 'PROCESSED'),
(18, 16,4, 'DATA',  '2024-06-05 09:00:00', 0,   40960, '61400660003', NULL,          0, '2024-06-05 09:30:00', 'PROCESSED'),
(19, 17,1, 'SMS',   '2024-06-05 10:00:00', 0,   0,     '61400770001', '61477770001', 0, '2024-06-05 10:01:00', 'PROCESSED'),
(20, 18,4, 'DATA',  '2024-06-06 08:00:00', 0,   15360, '61400880001', NULL,          0, '2024-06-06 08:10:00', 'PROCESSED'),
(21, 19,1, 'VOICE', '2024-06-06 09:00:00', 75,  0,     '61400990001', '61488880001', 0, '2024-06-06 09:02:00', 'PROCESSED'),
(22, 20,1, 'SMS',   '2024-06-06 10:00:00', 0,   0,     '61400220099', '61488880002', 0, '2024-06-06 10:01:00', 'PROCESSED'),
(23, 1, 1, 'VOICE', '2024-06-07 09:00:00', 200, 0,     '61400100001', '61411110004', 0, '2024-06-07 09:05:00', 'PROCESSED'),
(24, 4, 3, 'VOICE', '2024-06-07 10:00:00', 50,  0,     '61400110001', '61422220003', 0, '2024-06-07 10:02:00', 'PROCESSED'),
(25, 5, 4, 'DATA',  '2024-06-07 11:00:00', 0,   -512,  '61400110002', NULL,          0, '2024-06-07 11:05:00', 'SUSPECT'), -- VALIDITY negative
(26, 7, 2, 'DATA',  '2024-06-08 07:00:00', 0,   2560,  '61400220001', NULL,          0, '2024-06-08 07:04:00', 'PROCESSED'),
(27, 8, 1, 'VOICE', '2024-06-08 08:00:00', 110, 0,     '61400220002', '61433330003', 0, '2024-06-08 08:03:00', 'PROCESSED'),
(28, 11,3, 'VOICE', '2024-06-08 09:00:00', 320, 0,     '61400550001', '61444440003', 0, '2024-06-08 09:06:00', 'PROCESSED'),
(29, 14,3, 'VOICE', '2024-06-08 10:00:00', 400, 0,     '61400660001', '61466660003', 0, '2024-06-08 10:08:00', 'PROCESSED'),
(30, 15,5, 'VOICE', '2024-06-09 08:00:00', 70,  0,     '61400660002', '61499990001', 1, '2024-06-09 08:02:00', 'PROCESSED'),
(31, 16,4, 'DATA',  '2024-06-09 09:00:00', 0,   12288, '61400660003', NULL,          0, '2024-06-09 09:12:00', 'PROCESSED'),
(32, 3, 1, 'SMS',   '2024-06-09 10:00:00', 0,   0,     '61400100002', '61411110005', 0, '2024-06-09 10:01:00', 'PROCESSED'),
(33, 9, 4, 'DATA',  '2024-06-10 08:00:00', 0,   30720, '61400330001', NULL,          0, '2024-06-10 08:20:00', 'PROCESSED'),
(34, 18,4, 'DATA',  '2024-06-10 09:00:00', 0,   7680,  '61400880001', NULL,          0, '2024-06-10 09:08:00', 'PROCESSED'),
(35, 19,1, 'VOICE', '2024-06-10 10:00:00', 40,  0,     '61400990001', '61488880003', 0, '2024-06-10 10:01:00', 'PROCESSED'),
(36, 1, 1, 'VOICE', '2024-06-11 09:00:00', 95,  0,     '61400100001', '61411110006', 0, '2024-06-11 09:03:00', 'PROCESSED'),
(37, 4, 3, 'VOICE', '2024-06-11 10:00:00', 140, 0,     '61400110001', '61422220004', 0, '2024-06-11 10:04:00', 'PROCESSED'),
(38, 6, 3, 'VOICE', '2024-06-11 11:00:00', 55,  0,     '61400110003', '61422220005', 0, '2024-06-11 11:02:00', 'PROCESSED'),
(39, 12,1, 'SMS',   '2024-06-11 12:00:00', 0,   0,     '61400550002', '61444440004', 0, '2024-06-11 12:01:00', 'PROCESSED'),
(40, 20,1, 'SMS',   '2024-06-11 13:00:00', 0,   0,     '61400220099', '61488880004', 0, '2024-06-11 13:01:00', 'PROCESSED'),
-- UNIQUENESS: intentional duplicate of cdr 1 pattern
(41, 1, 1, 'VOICE', '2024-06-01 09:00:00', 120, 0,     '61400100001', '61411110001', 0, '2024-06-01 09:05:00', 'DUPLICATE'),
(42, 2, 4, 'DATA',  '2024-06-12 08:00:00', 0,   6400,  '61400100001', NULL,          0, '2024-06-12 08:05:00', 'PROCESSED'),
(43, 5, 4, 'DATA',  '2024-06-12 09:00:00', 0,   25600, '61400110002', NULL,          0, '2024-06-12 09:10:00', 'PROCESSED'),
(44, 7, 2, 'DATA',  '2024-06-12 10:00:00', 0,   1024,  '61400220001', NULL,          0, '2024-06-12 10:02:00', 'PROCESSED'),
(45, 11,3,'VOICE','2024-06-12 11:00:00', 180, 0,     '61400550001', '61444440005', 0, '2024-06-12 11:04:00', 'PROCESSED'),
(46, 14,3,'VOICE','2024-06-13 08:00:00', 250, 0,     '61400660001', '61466660004', 0, '2024-06-13 08:05:00', 'PROCESSED'),
(47, 15,1,'VOICE','2024-06-13 09:00:00', 80,  0,     '61400660002', '61466660005', 1, '2024-06-13 09:02:00', 'PROCESSED'),
-- TIMELINESS: mediation before usage start
(48, 16,4,'DATA', '2024-06-13 10:00:00', 0,   5000,  '61400660003', NULL,          0, '2024-06-13 09:50:00', 'SUSPECT'),
(49, 17,1,'SMS',  '2024-06-13 11:00:00', 0,   0,     '61400770001', '61477770002', 0, '2024-06-13 11:01:00', 'PROCESSED'),
(50, 19,1,'VOICE','2024-06-13 12:00:00', 65,  0,     '61400990001', '61488880005', 0, '2024-06-13 12:02:00', 'PROCESSED');

-- ---------------------------------------------------------------------------
-- Rated usage (~45) — leave CDRs 6,12,15,25,41,48 unrated (completeness)
-- Accuracy outlier on rated_usage for cdr 5
-- ---------------------------------------------------------------------------
INSERT INTO rated_usage (rated_usage_id, cdr_id, product_id, rated_amount, rate_plan_applied, rating_timestamp, rating_status) VALUES
(1,  1,  1, 0.5000, 'VOICE_ANY', '2024-06-01 09:10:00', 'RATED'),
(2,  2,  1, 0.2500, 'VOICE_ANY', '2024-06-01 10:10:00', 'RATED'),
(3,  3,  2, 39.9900,'DATA_UL_L','2024-06-01 11:20:00', 'RATED'),
(4,  4,  3, 0.1000, 'SMS_500',  '2024-06-01 12:10:00', 'RATED'),
(5,  5,  1, 99.9900,'VOICE_ANY','2024-06-02 08:20:00', 'RATED'), -- ACCURACY outlier (should be ~1.25)
(6,  7,  6, 29.9900,'DATA_50',  '2024-06-02 10:30:00', 'RATED'),
(7,  8,  4, 0.7500, 'BIZ_PLUS', '2024-06-02 11:15:00', 'RATED'),
(8,  9,  4, 0.5000, 'BIZ_PLUS', '2024-06-03 07:20:00', 'RATED'),
(9,  10, 1, 0.7500, 'VOICE_ANY','2024-06-03 08:15:00', 'RATED'),
(10, 11, 2, 39.9900,'DATA_UL_L','2024-06-03 09:40:00', 'RATED'),
(11, 13, 4, 1.0000, 'BIZ_PLUS', '2024-06-04 08:20:00', 'RATED'),
(12, 14, 7, 0.2200, 'SMS_PAYG', '2024-06-04 09:10:00', 'RATED'),
(13, 16, 4, 2.5000, 'BIZ_PLUS', '2024-06-05 07:30:00', 'RATED'),
(14, 17, 5, 2.2500, 'ROAM_VA',  '2024-06-05 08:20:00', 'RATED'),
(15, 18, 6, 29.9900,'DATA_50',  '2024-06-05 09:45:00', 'RATED'),
(16, 19, 3, 0.1000, 'SMS_500',  '2024-06-05 10:10:00', 'RATED'),
(17, 20, 2, 39.9900,'DATA_UL_L','2024-06-06 08:25:00', 'RATED'),
(18, 21, 1, 0.3125, 'VOICE_ANY','2024-06-06 09:15:00', 'RATED'),
(19, 22, 7, 0.2200, 'SMS_PAYG', '2024-06-06 10:10:00', 'RATED'),
(20, 23, 1, 0.8333, 'VOICE_ANY','2024-06-07 09:20:00', 'RATED'),
(21, 24, 1, 0.2083, 'VOICE_ANY','2024-06-07 10:15:00', 'RATED'),
(22, 26, 4, 0.2500, 'BIZ_PLUS', '2024-06-08 07:20:00', 'RATED'),
(23, 27, 1, 0.4583, 'VOICE_ANY','2024-06-08 08:15:00', 'RATED'),
(24, 28, 4, 1.3333, 'BIZ_PLUS', '2024-06-08 09:20:00', 'RATED'),
(25, 29, 4, 1.6667, 'BIZ_PLUS', '2024-06-08 10:25:00', 'RATED'),
(26, 30, 5, 1.7500, 'ROAM_VA',  '2024-06-09 08:15:00', 'RATED'),
(27, 31, 6, 29.9900,'DATA_50',  '2024-06-09 09:30:00', 'RATED'),
(28, 32, 3, 0.1000, 'SMS_500',  '2024-06-09 10:10:00', 'RATED'),
(29, 33, 2, 39.9900,'DATA_UL_L','2024-06-10 08:40:00', 'RATED'),
(30, 34, 2, 39.9900,'DATA_UL_L','2024-06-10 09:20:00', 'RATED'),
(31, 35, 1, 0.1667, 'VOICE_ANY','2024-06-10 10:10:00', 'RATED'),
(32, 36, 1, 0.3958, 'VOICE_ANY','2024-06-11 09:15:00', 'RATED'),
(33, 37, 1, 0.5833, 'VOICE_ANY','2024-06-11 10:15:00', 'RATED'),
(34, 38, 4, 0.4583, 'BIZ_PLUS', '2024-06-11 11:15:00', 'RATED'),
(35, 39, 7, 0.2200, 'SMS_PAYG', '2024-06-11 12:10:00', 'RATED'),
(36, 40, 7, 0.2200, 'SMS_PAYG', '2024-06-11 13:10:00', 'RATED'),
(37, 42, 2, 39.9900,'DATA_UL_L','2024-06-12 08:20:00', 'RATED'),
(38, 43, 6, 29.9900,'DATA_50',  '2024-06-12 09:25:00', 'RATED'),
(39, 44, 4, 0.1000, 'BIZ_PLUS', '2024-06-12 10:15:00', 'RATED'),
(40, 45, 4, 0.7500, 'BIZ_PLUS', '2024-06-12 11:20:00', 'RATED'),
(41, 46, 4, 1.0417, 'BIZ_PLUS', '2024-06-13 08:20:00', 'RATED'),
(42, 47, 5, 2.0000, 'ROAM_VA',  '2024-06-13 09:15:00', 'RATED'),
(43, 49, 3, 0.1000, 'SMS_500',  '2024-06-13 11:10:00', 'RATED'),
(44, 50, 1, 0.2708, 'VOICE_ANY','2024-06-13 12:15:00', 'RATED');

-- ---------------------------------------------------------------------------
-- Invoices + lines + payments — includes consistency + timeliness defects
-- ---------------------------------------------------------------------------
INSERT INTO invoice (invoice_id, account_id, billing_period_start, billing_period_end, invoice_date, total_amount, invoice_status) VALUES
(1, 1,  '2024-06-01', '2024-06-30', '2024-07-01', 41.07,  'SENT'),
(2, 3,  '2024-06-01', '2024-06-30', '2024-07-01', 170.00, 'SENT'), -- CONSISTENCY: will not match lines (~131.22)
(3, 5,  '2024-06-01', '2024-06-30', '2024-07-01', 1.50,   'PAID'),
(4, 8,  '2024-06-01', '2024-06-30', '2024-07-01', 2.47,   'OVERDUE'), -- TIMELINESS: overdue unpaid
(5, 10, '2024-06-01', '2024-06-30', '2024-07-01', 7.50,   'GENERATED'),
(6, 14, '2024-06-01', '2024-06-30', '2024-07-01', 0.58,   'SENT');

INSERT INTO invoice_line_item (line_item_id, invoice_id, rated_usage_id, subscription_id, charge_type, amount, description) VALUES
(1, 1, 1,  1, 'USAGE',     0.50,  'Voice call 120s'),
(2, 1, 2,  1, 'USAGE',     0.25,  'Voice call 60s'),
(3, 1, 3,  2, 'RECURRING', 39.99, 'Data Unlimited Lite'),
(4, 1, 20, 1, 'USAGE',     0.83,  'Voice call 200s'),
(5, 2, 5,  4, 'USAGE',     99.99, 'Voice outlier charge'),
(6, 2, 6,  5, 'RECURRING', 29.99, 'Data Turbo 50GB'),
(7, 2, 7,  6, 'USAGE',     0.75,  'Bundle voice'),
(8, 2, NULL,4,'RECURRING', 0.49,  'Admin fee'),
(9, 3, 8,  7, 'USAGE',     0.50,  'Data usage share'),
(10,3, 9,  8, 'USAGE',     0.75,  'Voice 180s'),
(11,3, 22, 7, 'USAGE',     0.25,  'Data share'),
(12,4, 11, 11,'USAGE',     1.00,  'Business voice'),
(13,4, 12, 12,'USAGE',     0.22,  'SMS PayG'),
(14,4, 24, 11,'USAGE',     1.25,  'Business voice'),
(15,5, 13, 14,'USAGE',     2.50,  'Corporate voice'),
(16,5, 14, 15,'USAGE',     2.25,  'Roam voice'),
(17,5, 25, 14,'USAGE',     1.67,  'Corporate voice'),
(18,5, 26, 15,'USAGE',     1.08,  'Roam voice adj'),
(19,6, 18, 19,'USAGE',     0.31,  'Consumer voice'),
(20,6, 31, 19,'USAGE',     0.17,  'Consumer voice'),
(21,6, 44, 19,'USAGE',     0.27,  'Consumer voice');

INSERT INTO payment (payment_id, account_id, invoice_id, payment_date, payment_amount, payment_method, payment_status) VALUES
(1, 1,  1, '2024-07-05', 41.07, 'CARD',          'SUCCESS'),
(2, 5,  3, '2024-07-03', 1.50,  'DIRECT_DEBIT',  'SUCCESS'),
(3, 10, 5, '2024-07-10', 5.00,  'BANK_TRANSFER', 'PENDING'), -- partial / incomplete collections
(4, 14, 6, '2024-07-08', 0.58,  'CARD',          'SUCCESS');
-- invoice 2 and 4 left without successful payment (leakage / overdue)
