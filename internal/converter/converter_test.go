package converter

import (
	"strings"
	"testing"

	"github.com/mashuiping/sqlkit/internal/parser"
)

func TestMySQLToPostgresConverter_ConvertSQL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:  "converts ignore insert to conflict do nothing",
			input: "INSERT IGNORE INTO users (id, name) VALUES (1, 'Ada');",
			contains: []string{
				"INSERT INTO \"users\" (\"id\", \"name\") VALUES (1, 'Ada') ON CONFLICT DO NOTHING;",
			},
		},
		{
			name:  "keeps functions containing FROM in the select list",
			input: "SELECT IFNULL(name, 'unknown'), DATE_FORMAT(created_at, '%Y-%m-%d'), FROM_UNIXTIME(updated_at) FROM users LIMIT 10, 20;",
			contains: []string{
				"COALESCE(name, 'unknown')",
				"TO_CHAR(CAST(created_at AS TIMESTAMP), 'YYYY-MM-DD')",
				"TO_TIMESTAMP(updated_at)",
				"FROM users LIMIT 20 OFFSET 10;",
			},
		},
		{
			name:  "converts group concat without an ordering clause",
			input: "SELECT GROUP_CONCAT(DISTINCT name SEPARATOR ',') FROM users;",
			contains: []string{
				"STRING_AGG(DISTINCT name, ',')",
			},
		},
		{
			name:  "preserves update target alias",
			input: "UPDATE users u JOIN accounts a ON a.user_id = u.id SET u.status = 'active' WHERE a.enabled = 1;",
			contains: []string{
				"UPDATE \"users\" AS \"u\"",
				"FROM \"accounts\" \"a\"",
				"WHERE a.user_id = u.id AND a.enabled = 1;",
			},
		},
		{
			name:  "converts enum unsigned foreign key and check constraints",
			input: "CREATE TABLE orders (id INT UNSIGNED NOT NULL, status ENUM('new', 'paid'), user_id INT, CONSTRAINT fk_order_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE, CHECK (id > 0));",
			contains: []string{
				"\"id\" BIGINT NOT NULL",
				"\"status\" TEXT CHECK (\"status\" IN ('new', 'paid'))",
				"CONSTRAINT \"fk_order_user\" FOREIGN KEY (user_id) REFERENCES \"users\" (id) ON DELETE CASCADE",
				"CHECK (id > 0)",
			},
		},
	}

	converter := NewConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converted, err := converter.ConvertSQL(tt.input)
			if err != nil {
				t.Fatalf("ConvertSQL() error = %v", err)
			}
			for _, expected := range tt.contains {
				if !strings.Contains(converted, expected) {
					t.Errorf("ConvertSQL() = %q, missing %q", converted, expected)
				}
			}
			if _, err := parser.ParsePostgreSQL(converted); err != nil {
				t.Errorf("ConvertSQL() produced invalid PostgreSQL: %v\nSQL: %s", err, converted)
			}
		})
	}
}

func TestMySQLToPostgresConverter_ConvertSQLRejectsUnsafeSyntax(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "on duplicate key update without a conflict target",
			input: "INSERT INTO users (id, name) VALUES (1, 'Ada') ON DUPLICATE KEY UPDATE name = VALUES(name);",
		},
		{
			name:  "multi table delete",
			input: "DELETE u FROM users u JOIN sessions s ON s.user_id = u.id WHERE s.expires_at < NOW();",
		},
		{
			name:  "ordered group concat",
			input: "SELECT GROUP_CONCAT(name ORDER BY name SEPARATOR ',') FROM users;",
		},
	}

	converter := NewConverter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := converter.ConvertSQL(tt.input); err == nil {
				t.Fatal("ConvertSQL() error = nil, want an error")
			}
		})
	}
}
