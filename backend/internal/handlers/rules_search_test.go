package handlers

import "testing"

func TestMatchAndDuplicate(t *testing.T) {
	rules := []storedRule{
		{ID: 1, Name: "CDR msisdn present", Dimension: "COMPLETENESS", Business: "Every CDR has an msisdn", Tables: `["usage_cdr"]`, CheckSQL: "SELECT * FROM usage_cdr WHERE msisdn IS NULL", Status: "ACTIVE"},
		{ID: 2, Name: "Invoice total", Dimension: "ACCURACY", Business: "Invoice amount matches lines", Tables: `["invoice"]`, CheckSQL: "SELECT * FROM invoice WHERE amount < 0", Status: "RETIRED"},
	}
	got := matchRules(rules, ruleQuery{Q: "completeness on usage_cdr", Limit: 8})
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("search: %+v", got)
	}
	if _, reason, ok := findDuplicate(rules, "Other name", "COMPLETENESS", "select * from usage_cdr where msisdn is null"); !ok || reason != "check" {
		t.Fatal("expected check-sql duplicate")
	}
	if _, _, ok := findDuplicate(rules, "CDR msisdn present", "ACCURACY", "SELECT 1"); !ok {
		t.Fatal("expected name duplicate")
	}
}
