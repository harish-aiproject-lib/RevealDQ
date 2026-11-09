package sqlguard

import "testing"

func TestValidateSelect(t *testing.T) {
	ok := []string{
		"SELECT * FROM usage_cdr WHERE msisdn IS NULL",
		"  select a, b from t;  ",
		"WITH x AS (SELECT 1 AS n) SELECT n FROM x",
		"SELECT * FROM t WHERE note = 'please delete'",
		"SELECT REPLACE(name, 'a', 'b') FROM t",
	}
	for _, s := range ok {
		if err := ValidateSelect(s); err != nil {
			t.Errorf("want ok %q: %v", s, err)
		}
	}
	bad := []string{
		"",
		"UPDATE t SET a=1",
		"SELECT 1; DROP TABLE t",
		"SELECT * FROM t INTO OUTFILE '/tmp/x'",
		"SELECT * FROM t FOR UPDATE",
		"SELECT LOAD_FILE('/etc/passwd')",
		"DELETE FROM t",
		"SELECT 1 /* ; */ ; DROP TABLE t",
	}
	for _, s := range bad {
		if err := ValidateSelect(s); err == nil {
			t.Errorf("want reject %q", s)
		}
	}
}
