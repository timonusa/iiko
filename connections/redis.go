package connections

import (
	"net"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// NewRedis создаёт клиент Redis. Клиент подключается лениво, как и пул PostgreSQL.
func NewRedis() *redis.Client {
	db, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
	return redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT")),
		Username: os.Getenv("REDIS_USER"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       db,
	})
}
