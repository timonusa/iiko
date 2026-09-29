package handlers

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Подключения для всех хэндлеров; задаются в main до запуска сервера.
var (
	DB    *pgxpool.Pool
	Redis *redis.Client
)

type ResponseT struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   string      `json:"error"`
}

type RegisterRequestT struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterResponseT struct {
	Token string `json:"token"`
}

type LoginRequestT struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponseT struct {
	Token string `json:"token"`
}

type MeResponseT struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}
