USE RevealDQ;

DROP PROCEDURE IF EXISTS sp_dq_cdr_unrated;

DELIMITER $$
CREATE PROCEDURE sp_dq_cdr_unrated()
BEGIN
    SELECT
        c.cdr_id,
        c.subscription_id,
        c.usage_type,
        c.usage_start_timestamp,
        c.cdr_status,
        'COMPLETENESS' AS dimension,
        'CDR has no corresponding rated_usage row' AS finding
    FROM usage_cdr c
    LEFT JOIN rated_usage r ON r.cdr_id = c.cdr_id
    WHERE r.rated_usage_id IS NULL
      AND c.cdr_status <> 'DUPLICATE'
    ORDER BY c.cdr_id;

    SELECT COUNT(*) AS rows_checked FROM usage_cdr WHERE cdr_status <> 'DUPLICATE';
    SELECT COUNT(*) AS rows_failed
    FROM usage_cdr c
    LEFT JOIN rated_usage r ON r.cdr_id = c.cdr_id
    WHERE r.rated_usage_id IS NULL
      AND c.cdr_status <> 'DUPLICATE';
END$$
DELIMITER ;
