package parser

import "testing"

func TestPostgreSQLParser_Parse(t *testing.T) {
	tests := []struct {
		name           string
		sql            string
		wantStatements int
		wantErr        bool
	}{
		{
			name:           "postgres array containment",
			sql:            "SELECT ARRAY[1, 2, 3] @> ARRAY[1, 2];",
			wantStatements: 1,
		},
		{
			name:           "multiple statements",
			sql:            "CREATE TABLE accounts (id INT PRIMARY KEY); SELECT * FROM accounts;",
			wantStatements: 2,
		},
		{
			name:    "invalid syntax",
			sql:     "SELECT FROM;",
			wantErr: true,
		},
		{
			name:    "empty SQL",
			sql:     " \n\t ",
			wantErr: true,
		},
	}

	postgresParser := NewPostgreSQLParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statements, err := postgresParser.Parse(tt.sql)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Parse() error = nil, want an error")
				}
				return
			}

			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(statements) != tt.wantStatements {
				t.Errorf("Parse() returned %d statements, want %d", len(statements), tt.wantStatements)
			}
		})
	}
}

func TestParsePostgreSQL(t *testing.T) {
	statements, err := ParsePostgreSQL("SELECT $1::TEXT;")
	if err != nil {
		t.Fatalf("ParsePostgreSQL() error = %v", err)
	}
	if len(statements) != 1 {
		t.Fatalf("ParsePostgreSQL() returned %d statements, want 1", len(statements))
	}
	if statements[0].NumPlaceholders != 1 {
		t.Errorf("NumPlaceholders = %d, want 1", statements[0].NumPlaceholders)
	}
}
