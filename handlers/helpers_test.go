package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"service/connections"
)

// infraReady показывает, доступны ли PostgreSQL и Redis для интеграционных тестов.
var infraReady bool

// TestMain подключается к PostgreSQL и Redis по тем же переменным окружения, что и сервис.
// Если хранилища недоступны, интеграционные тесты пропускаются, а юнит-тесты выполняются.
func TestMain(m *testing.M) {
	if os.Getenv("POSTGRES_HOST") != "" && os.Getenv("REDIS_HOST") != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pool, err := connections.NewPostgres(ctx)
		if err == nil {
			err = pool.Ping(ctx)
		}
		rdb := connections.NewRedis()
		if err == nil {
			err = rdb.Ping(ctx).Err()
		}
		cancel()
		if err != nil {
			log.Printf("integration tests disabled: %v", err)
		} else {
			DB, Redis, infraReady = pool, rdb, true
		}
	}

	code := m.Run()

	if DB != nil {
		DB.Close()
	}
	if Redis != nil {
		_ = Redis.Close()
	}
	os.Exit(code)
}

// requireInfra пропускает тест, если PostgreSQL и Redis недоступны.
func requireInfra(t *testing.T) {
	t.Helper()
	if !infraReady {
		t.Skip("PostgreSQL/Redis unavailable: set POSTGRES_* and REDIS_* to run integration tests")
	}
}

// swapDB подменяет пул PostgreSQL на время теста и восстанавливает его после.
func swapDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	prev := DB
	DB = pool
	t.Cleanup(func() { DB = prev })
}

// swapRedis подменяет клиент Redis на время теста и восстанавливает его после.
func swapRedis(t *testing.T, rdb *redis.Client) {
	t.Helper()
	prev := Redis
	Redis = rdb
	t.Cleanup(func() { Redis = prev })
}

// deadPostgres возвращает пул, указывающий на закрытый порт: любой запрос завершится ошибкой.
func deadPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// deadRedis возвращает клиент, указывающий на закрытый порт: любая команда завершится ошибкой.
func deadRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: time.Second})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

// call выполняет запрос к хэндлеру и возвращает статус, разобранный ответ и сырое тело.
// Заодно проверяет, что ответ — JSON в формате ResponseT.
func call(t *testing.T, h http.HandlerFunc, method, body, auth string) (int, ResponseT, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, "/", strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var resp ResponseT
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not JSON: %v (body %q)", err, rec.Body.String())
	}
	if resp.Success != (resp.Error == "") {
		t.Errorf("inconsistent response: success=%v error=%q", resp.Success, resp.Error)
	}
	return rec.Code, resp, rec.Body.String()
}

// credentials возвращает JSON-тело запроса с email и паролем.
func credentials(email, password string) string {
	b, _ := json.Marshal(map[string]string{"email": email, "password": password})
	return string(b)
}

// uniqueEmail возвращает email, которого ещё нет в БД, и удаляет пользователя после теста.
func uniqueEmail(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	email := fmt.Sprintf("test-%s@example.com", hex.EncodeToString(b))
	t.Cleanup(func() {
		if DB != nil {
			_, _ = DB.Exec(context.Background(), `DELETE FROM users WHERE email = $1`, email)
		}
	})
	return email
}

// dataField достаёт строковое поле из Data успешного ответа.
func dataField(t *testing.T, resp ResponseT, key string) string {
	t.Helper()
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is %T, want object", resp.Data)
	}
	v, ok := data[key].(string)
	if !ok || v == "" {
		t.Fatalf("data.%s = %v, want non-empty string", key, data[key])
	}
	return v
}

// register регистрирует пользователя и возвращает токен.
func register(t *testing.T, email, password string) string {
	t.Helper()
	status, resp, body := call(t, Register, http.MethodPost, credentials(email, password), "")
	if status != http.StatusCreated {
		t.Fatalf("register: status = %d, body %s", status, body)
	}
	return dataField(t, resp, "token")
}
