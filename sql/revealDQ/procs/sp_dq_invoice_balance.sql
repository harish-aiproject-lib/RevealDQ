USE RevealDQ;

DROP PROCEDURE IF EXISTS sp_dq_invoice_balance;

DELIMITER $$
CREATE PROCEDURE sp_dq_invoice_balance()
BEGIN
    SELECT
        i.invoice_id,
        i.account_id,
        i.total_amount AS invoice_total,
        COALESCE(SUM(li.amount), 0) AS line_total,
        ROUND(i.total_amount - COALESCE(SUM(li.amount), 0), 2) AS variance,
        'CONSISTENCY' AS dimension,
        'invoice.total_amount does not equal sum of invoice_line_item.amount' AS finding
    FROM invoice i
    LEFT JOIN invoice_line_item li ON li.invoice_id = i.invoice_id
    GROUP BY i.invoice_id, i.account_id, i.total_amount
    HAVING ABS(i.total_amount - COALESCE(SUM(li.amount), 0)) > 0.01
    ORDER BY i.invoice_id;

    SELECT COUNT(*) AS rows_checked FROM invoice;
    SELECT COUNT(*) AS rows_failed FROM (
        SELECT i.invoice_id
        FROM invoice i
        LEFT JOIN invoice_line_item li ON li.invoice_id = i.invoice_id
        GROUP BY i.invoice_id, i.total_amount
        HAVING ABS(i.total_amount - COALESCE(SUM(li.amount), 0)) > 0.01
    ) x;
END$$
DELIMITER ;
