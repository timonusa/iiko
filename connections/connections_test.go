package connections

import (
	"context"
	"testing"
)

// TestNewPostgres проверяет, что строка подключения собирается из окружения
// и спецсимволы в пароле не ломают её. Пул ленивый, поэтому PostgreSQL не нужен.
func TestNewPostgres(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "db.local")
	t.Setenv("POSTGRES_PORT", "6543")
	t.Setenv("POSTGRES_USER", "auth")
	t.Setenv("POSTGRES_PASSWORD", "p@ss:w/o?r#d%")
	t.Setenv("POSTGRES_DB", "authdb")
	t.Setenv("POSTGRES_SSLMODE", "disable")

	pool, err := NewPostgres(context.Background())
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	defer pool.Close()

	cfg := pool.Config().ConnConfig
	if cfg.Host != "db.local" || cfg.Port != 6543 {
		t.Errorf("addr = %s:%d, want db.local:6543", cfg.Host, cfg.Port)
	}
	if cfg.User != "auth" || cfg.Password != "p@ss:w/o?r#d%" || cfg.Database != "authdb" {
		t.Errorf("user/password/db = %q/%q/%q", cfg.User, cfg.Password, cfg.Database)
	}
	if cfg.TLSConfig != nil {
		t.Errorf("TLS is enabled with sslmode=disable")
	}
}

// TestNewPostgresInvalidConfig проверяет, что некорректная конфигурация возвращает ошибку.
func TestNewPostgresInvalidConfig(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "db.local")
	t.Setenv("POSTGRES_PORT", "not-a-port")
	t.Setenv("POSTGRES_SSLMODE", "disable")

	if pool, err := NewPostgres(context.Background()); err == nil {
		pool.Close()
		t.Fatal("NewPostgres with invalid port: want error, got nil")
	}
}

// TestNewRedis проверяет, что параметры клиента берутся из окружения,
// а некорректный REDIS_DB превращается в 0.
func TestNewRedis(t *testing.T) {
	tests := []struct {
		name   string
		db     string
		wantDB int
	}{
		{"numeric db", "3", 3},
		{"invalid db", "abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("REDIS_HOST", "127.0.0.1")
			t.Setenv("REDIS_PORT", "6380")
			t.Setenv("REDIS_USER", "default")
			t.Setenv("REDIS_PASSWORD", "secret")
			t.Setenv("REDIS_DB", tt.db)

			rdb := NewRedis()
			defer func() { _ = rdb.Close() }()

			opt := rdb.Options()
			if opt.Addr != "127.0.0.1:6380" {
				t.Errorf("Addr = %q, want 127.0.0.1:6380", opt.Addr)
			}
			if opt.Username != "default" || opt.Password != "secret" {
				t.Errorf("Username/Password = %q/%q", opt.Username, opt.Password)
			}
			if opt.DB != tt.wantDB {
				t.Errorf("DB = %d, want %d", opt.DB, tt.wantDB)
			}
		})
	}
}
