USE RevealDQ;

DROP PROCEDURE IF EXISTS sp_dq_msisdn_validity;

DELIMITER $$
CREATE PROCEDURE sp_dq_msisdn_validity()
BEGIN
    SELECT
        s.subscription_id,
        s.account_id,
        s.service_identifier,
        s.subscription_status,
        'VALIDITY' AS dimension,
        'service_identifier is not a valid AU MSISDN (expected 61XXXXXXXXX digits)' AS finding
    FROM service_subscription s
    WHERE s.service_identifier NOT REGEXP '^61[0-9]{9}$'
    ORDER BY s.subscription_id;

    SELECT COUNT(*) AS rows_checked FROM service_subscription;
    SELECT COUNT(*) AS rows_failed
    FROM service_subscription s
    WHERE s.service_identifier NOT REGEXP '^61[0-9]{9}$';
END$$
DELIMITER ;
