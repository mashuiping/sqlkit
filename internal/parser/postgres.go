// Package parser provides SQL parsers used by SQLKit.
package parser

import (
	"fmt"
	"strings"

	pgparser "github.com/auxten/postgresql-parser/pkg/sql/parser"
)

// PostgreSQLParser parses PostgreSQL SQL into the parser's native AST.
//
// The returned Statements value retains each statement's AST, original SQL,
// and placeholder metadata. This keeps SQLKit's parsing boundary independent
// from the converter's MySQL-specific AST.
type PostgreSQLParser struct{}

// NewPostgreSQLParser creates a PostgreSQL parser.
func NewPostgreSQLParser() *PostgreSQLParser {
	return &PostgreSQLParser{}
}

// Parse parses one or more semicolon-separated PostgreSQL statements.
func (p *PostgreSQLParser) Parse(sql string) (pgparser.Statements, error) {
	if strings.TrimSpace(sql) == "" {
		return nil, fmt.Errorf("PostgreSQL SQL is empty")
	}

	statements, err := pgparser.Parse(sql)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL SQL: %w", err)
	}

	return statements, nil
}

// ParsePostgreSQL parses one or more PostgreSQL statements using the default parser.
func ParsePostgreSQL(sql string) (pgparser.Statements, error) {
	return NewPostgreSQLParser().Parse(sql)
}
