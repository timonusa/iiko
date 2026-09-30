package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ---------- юнит-тесты: валидация, БД и Redis не нужны ----------

// TestRegisterValidation проверяет отказ на невалидное тело, email и пароль.
func TestRegisterValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"broken json", "{", "invalid JSON body"},
		{"empty body", "", "invalid JSON body"},
		{"empty email", credentials("", "password123"), "invalid email"},
		{"no at sign", credentials("user.example.com", "password123"), "invalid email"},
		{"display name", credentials("User <user@example.com>", "password123"), "invalid email"},
		{"too long email", credentials(strings.Repeat("a", 250)+"@example.com", "password123"), "invalid email"},
		{"short password", credentials("user@example.com", "1234567"), "password must be 8-72 bytes long"},
		{"long password", credentials("user@example.com", strings.Repeat("p", 73)), "password must be 8-72 bytes long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, resp, _ := call(t, Register, http.MethodPost, tt.body, "")
			if status != http.StatusBadRequest || resp.Error != tt.wantErr {
				t.Fatalf("got %d %q, want 400 %q", status, resp.Error, tt.wantErr)
			}
		})
	}
}

// TestLoginValidation проверяет отказ на невалидное тело и пустые поля.
func TestLoginValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"broken json", "{", "invalid JSON body"},
		{"empty object", "{}", "email and password are required"},
		{"empty password", credentials("user@example.com", ""), "email and password are required"},
		{"empty email", credentials("", "password123"), "email and password are required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, resp, _ := call(t, Login, http.MethodPost, tt.body, "")
			if status != http.StatusBadRequest || resp.Error != tt.wantErr {
				t.Fatalf("got %d %q, want 400 %q", status, resp.Error, tt.wantErr)
			}
		})
	}
}

// TestMeAuthorizationHeader проверяет отказ при отсутствующем или некорректном заголовке.
func TestMeAuthorizationHeader(t *testing.T) {
	for _, header := range []string{"", "token", "Basic dXNlcjpwYXNz", "Bearer", "Bearer    "} {
		t.Run(header, func(t *testing.T) {
			status, resp, _ := call(t, Me, http.MethodGet, "", header)
			if status != http.StatusUnauthorized || resp.Error != "missing or invalid authorization header" {
				t.Fatalf("got %d %q, want 401", status, resp.Error)
			}
		})
	}
}

// TestStorageUnavailable проверяет, что при недоступной БД или Redis отдаётся 503.
func TestStorageUnavailable(t *testing.T) {
	t.Run("register without db", func(t *testing.T) {
		swapDB(t, deadPostgres(t))
		status, _, _ := call(t, Register, http.MethodPost, credentials("user@example.com", "password123"), "")
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", status)
		}
	})
	t.Run("login without db", func(t *testing.T) {
		swapDB(t, deadPostgres(t))
		status, _, _ := call(t, Login, http.MethodPost, credentials("user@example.com", "password123"), "")
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", status)
		}
	})
	t.Run("me without redis", func(t *testing.T) {
		swapRedis(t, deadRedis(t))
		status, _, _ := call(t, Me, http.MethodGet, "", "Bearer some-token")
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", status)
		}
	})
}

// ---------- интеграционные тесты: нужны PostgreSQL и Redis ----------

// TestRegister проверяет успешную регистрацию: 201, рабочий токен, нижний регистр email
// и то, что пароль хранится только в виде bcrypt-хэша и не попадает в ответ.
func TestRegister(t *testing.T) {
	requireInfra(t)
	email := uniqueEmail(t)
	const password = "password123"

	status, resp, body := call(t, Register, http.MethodPost, credentials(strings.ToUpper(email), password), "")
	if status != http.StatusCreated || !resp.Success {
		t.Fatalf("status = %d, body %s", status, body)
	}
	if strings.Contains(body, password) || strings.Contains(body, "$2a$") {
		t.Fatalf("response leaks password or hash: %s", body)
	}
	token := dataField(t, resp, "token")

	var hash string
	err := DB.QueryRow(context.Background(), `SELECT password_hash FROM users WHERE email = $1`, email).Scan(&hash)
	if err != nil {
		t.Fatalf("user not stored with lowercased email: %v", err)
	}
	if hash == password || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		t.Fatalf("password_hash is not a bcrypt hash of the password")
	}

	status, resp, _ = call(t, Me, http.MethodGet, "", "Bearer "+token)
	if status != http.StatusOK || dataField(t, resp, "email") != email {
		t.Fatalf("me with register token: %d %+v", status, resp)
	}
}

// TestRegisterDuplicate проверяет 409 на повторный email, в том числе в другом регистре.
func TestRegisterDuplicate(t *testing.T) {
	requireInfra(t)
	email := uniqueEmail(t)
	register(t, email, "password123")

	for _, again := range []string{email, strings.ToUpper(email)} {
		status, resp, _ := call(t, Register, http.MethodPost, credentials(again, "otherpass123"), "")
		if status != http.StatusConflict || resp.Error != "user already exists" {
			t.Fatalf("register %q again: %d %q, want 409", again, status, resp.Error)
		}
	}
}

// TestLogin проверяет успешный вход: email нормализуется, выдаётся новый рабочий токен.
func TestLogin(t *testing.T) {
	requireInfra(t)
	email := uniqueEmail(t)
	regToken := register(t, email, "password123")

	status, resp, body := call(t, Login, http.MethodPost, credentials("  "+strings.ToUpper(email)+" ", "password123"), "")
	if status != http.StatusOK || !resp.Success {
		t.Fatalf("status = %d, body %s", status, body)
	}
	token := dataField(t, resp, "token")
	if token == regToken {
		t.Fatal("login returned the same token as register")
	}

	status, resp, _ = call(t, Me, http.MethodGet, "", "Bearer "+token)
	if status != http.StatusOK || dataField(t, resp, "email") != email {
		t.Fatalf("me with login token: %d %+v", status, resp)
	}
}

// TestLoginInvalidCredentials проверяет, что неверный пароль и несуществующий email
// дают одинаковый ответ 401 и не раскрывают, есть ли такой пользователь.
func TestLoginInvalidCredentials(t *testing.T) {
	requireInfra(t)
	email := uniqueEmail(t)
	register(t, email, "password123")

	wrongStatus, _, wrongBody := call(t, Login, http.MethodPost, credentials(email, "wrong-password"), "")
	unknownStatus, _, unknownBody := call(t, Login, http.MethodPost, credentials(uniqueEmail(t), "password123"), "")

	if wrongStatus != http.StatusUnauthorized || unknownStatus != http.StatusUnauthorized {
		t.Fatalf("statuses = %d/%d, want 401/401", wrongStatus, unknownStatus)
	}
	if wrongBody != unknownBody {
		t.Fatalf("responses differ: %s vs %s", wrongBody, unknownBody)
	}
}

// TestLoginRedisUnavailable проверяет 503, если пользователь найден, но сессию сохранить нельзя.
func TestLoginRedisUnavailable(t *testing.T) {
	requireInfra(t)
	email := uniqueEmail(t)
	register(t, email, "password123")

	swapRedis(t, deadRedis(t))
	status, _, _ := call(t, Login, http.MethodPost, credentials(email, "password123"), "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}

// TestMe проверяет ответ /me: id и email пользователя без пароля и его производных.
func TestMe(t *testing.T) {
	requireInfra(t)
	email := uniqueEmail(t)
	token := register(t, email, "password123")

	var id int64
	if err := DB.QueryRow(context.Background(), `SELECT id FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatalf("select id: %v", err)
	}

	// Схема Bearer регистронезависима.
	status, resp, body := call(t, Me, http.MethodGet, "", "bearer "+token)
	if status != http.StatusOK {
		t.Fatalf("status = %d, body %s", status, body)
	}
	data := resp.Data.(map[string]any)
	if len(data) != 2 || data["email"] != email || data["id"] != float64(id) {
		t.Fatalf("data = %v, want exactly {id: %d, email: %s}", data, id, email)
	}
}

// TestMeInvalidToken проверяет 401 для неизвестного, просроченного токена
// и токена удалённого пользователя.
func TestMeInvalidToken(t *testing.T) {
	requireInfra(t)

	t.Run("unknown", func(t *testing.T) {
		status, resp, _ := call(t, Me, http.MethodGet, "", "Bearer not-a-real-token")
		if status != http.StatusUnauthorized || resp.Error != "invalid or expired token" {
			t.Fatalf("got %d %q, want 401", status, resp.Error)
		}
	})

	t.Run("expired", func(t *testing.T) {
		t.Setenv("TOKEN_TTL", "1")
		token := register(t, uniqueEmail(t), "password123")
		time.Sleep(1500 * time.Millisecond)
		status, resp, _ := call(t, Me, http.MethodGet, "", "Bearer "+token)
		if status != http.StatusUnauthorized || resp.Error != "invalid or expired token" {
			t.Fatalf("got %d %q, want 401", status, resp.Error)
		}
	})

	t.Run("deleted user", func(t *testing.T) {
		email := uniqueEmail(t)
		token := register(t, email, "password123")
		if _, err := DB.Exec(context.Background(), `DELETE FROM users WHERE email = $1`, email); err != nil {
			t.Fatalf("delete user: %v", err)
		}
		status, resp, _ := call(t, Me, http.MethodGet, "", "Bearer "+token)
		if status != http.StatusUnauthorized || resp.Error != "invalid or expired token" {
			t.Fatalf("got %d %q, want 401", status, resp.Error)
		}
	})
}

// TestNewSession проверяет формат токена, ключ и значение в Redis и время жизни сессии.
func TestNewSession(t *testing.T) {
	requireInfra(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		ttlEnv  string
		wantTTL time.Duration
	}{
		{"custom ttl", "120", 120 * time.Second},
		{"default ttl when invalid", "abc", time.Hour},
		{"default ttl when non-positive", "0", time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TOKEN_TTL", tt.ttlEnv)
			const userID = 424242

			token, err := newSession(ctx, userID)
			if err != nil {
				t.Fatalf("newSession: %v", err)
			}
			if raw, err := base64.RawURLEncoding.DecodeString(token); err != nil || len(raw) != 32 {
				t.Fatalf("token %q is not 32 random bytes in base64url", token)
			}

			sum := sha256.Sum256([]byte(token))
			key := "session:" + hex.EncodeToString(sum[:])
			t.Cleanup(func() { Redis.Del(ctx, key) })

			if n, _ := Redis.Exists(ctx, "session:"+token).Result(); n != 0 {
				t.Fatal("raw token is stored in Redis")
			}
			val, err := Redis.Get(ctx, key).Result()
			if err != nil || val != strconv.Itoa(userID) {
				t.Fatalf("session value = %q (%v), want %d", val, err, userID)
			}
			ttl := Redis.TTL(ctx, key).Val()
			if ttl <= tt.wantTTL-5*time.Second || ttl > tt.wantTTL {
				t.Fatalf("ttl = %v, want about %v", ttl, tt.wantTTL)
			}
		})
	}

	t.Run("tokens are unique", func(t *testing.T) {
		a, errA := newSession(ctx, 1)
		b, errB := newSession(ctx, 1)
		if errA != nil || errB != nil || a == b {
			t.Fatalf("tokens %q %q (%v %v)", a, b, errA, errB)
		}
		for _, tok := range []string{a, b} {
			sum := sha256.Sum256([]byte(tok))
			Redis.Del(ctx, "session:"+hex.EncodeToString(sum[:]))
		}
	})

	t.Run("redis unavailable", func(t *testing.T) {
		swapRedis(t, deadRedis(t))
		if _, err := newSession(ctx, 1); err == nil {
			t.Fatal("want error, got nil")
		}
	})
}
