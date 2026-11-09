// Package sqlguard rejects anything that is not a single read-only SELECT
// (or a WITH … SELECT). It is the gate for rule checkSql and for agent queries.
package sqlguard

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineComment  = regexp.MustCompile(`(?m)--[^\n]*`)
	stringLit    = regexp.MustCompile(`'(?:''|\\'|[^'])*'`)
	forbidden    = regexp.MustCompile(`(?i)\b(insert|update|delete|drop|alter|create|grant|revoke|truncate|call|execute|prepare|deallocate|handler)\b|` +
		`\binto\s+(outfile|dumpfile)\b|\bfor\s+update\b|\block\s+in\s+share\s+mode\b|\bload_file\s*\(|\binto\s+@`)
)

// ValidateSelect returns nil when sql is one read-only SELECT/CTE.
func ValidateSelect(sql string) error {
	cleaned := stripComments(sql)
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimRight(cleaned, "; \t\r\n")
	if cleaned == "" {
		return fmt.Errorf("SQL is empty")
	}
	if strings.Contains(cleaned, ";") {
		return fmt.Errorf("only a single SELECT statement is allowed")
	}
	head := strings.ToLower(cleaned)
	if !strings.HasPrefix(head, "select") && !strings.HasPrefix(head, "with") {
		return fmt.Errorf("only SELECT statements are allowed")
	}
	masked := stringLit.ReplaceAllString(cleaned, "''")
	if loc := forbidden.FindStringIndex(masked); loc != nil {
		return fmt.Errorf("statement is not read-only (found %q)", strings.TrimSpace(masked[loc[0]:loc[1]]))
	}
	return nil
}

func stripComments(sql string) string {
	sql = blockComment.ReplaceAllString(sql, " ")
	sql = lineComment.ReplaceAllString(sql, " ")
	return sql
}
