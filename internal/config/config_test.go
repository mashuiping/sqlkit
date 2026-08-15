package config

import "testing"

func TestPostgresConfig(t *testing.T) {
	cfg := NewPostgresConfig()
	cfg.User = "postgres"
	cfg.Password = "secret"
	cfg.DBName = "sqlkit"

	wantDSN := "host=localhost port=5432 user=postgres password=secret dbname=sqlkit sslmode=disable"
	if got := cfg.PostgresDSN(); got != wantDSN {
		t.Errorf("PostgresDSN() = %q, want %q", got, wantDSN)
	}
	if got := cfg.DSN(DatabaseTypePostgres); got != wantDSN {
		t.Errorf("DSN(DatabaseTypePostgres) = %q, want %q", got, wantDSN)
	}
}

func TestTeledbConfig(t *testing.T) {
	cfg := NewTeledbConfig()
	cfg.User = "teledb"
	cfg.Password = "secret"
	cfg.DBName = "sqlkit"

	wantDSN := "host=localhost port=5432 user=teledb password=secret dbname=sqlkit sslmode=disable"
	if got := cfg.TeledbDSN(); got != wantDSN {
		t.Errorf("TeledbDSN() = %q, want %q", got, wantDSN)
	}
	if got := cfg.DSN(DatabaseTypeTeledb); got != wantDSN {
		t.Errorf("DSN(DatabaseTypeTeledb) = %q, want %q", got, wantDSN)
	}
}
