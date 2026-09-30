package handlers

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	DB    *pgxpool.Pool
	Redis *redis.Client
)

// ResponseT — общая обёртка всех ответов API.
type ResponseT struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   string      `json:"error"`
}

// RegisterRequestT — тело запроса POST /register.
type RegisterRequestT struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterResponseT — данные успешного ответа POST /register: токен новой сессии.
type RegisterResponseT struct {
	Token string `json:"token"`
}

// LoginRequestT — тело запроса POST /login.
type LoginRequestT struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponseT — данные успешного ответа POST /login: токен новой сессии.
type LoginResponseT struct {
	Token string `json:"token"`
}

// MeResponseT — данные успешного ответа GET /me: публичные поля пользователя.
type MeResponseT struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}
