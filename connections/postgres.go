package connections

import (
	"context"
	"net"
	"net/url"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgres создаёт пул подключений к PostgreSQL. Пул подключается лениво:
// если PostgreSQL недоступен, сервис всё равно стартует.
func NewPostgres(ctx context.Context) (*pgxpool.Pool, error) {
	// URL собирается через net/url, чтобы спецсимволы в пароле не ломали строку подключения.
	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD")),
		Host:     net.JoinHostPort(os.Getenv("POSTGRES_HOST"), os.Getenv("POSTGRES_PORT")),
		Path:     os.Getenv("POSTGRES_DB"),
		RawQuery: "sslmode=" + os.Getenv("POSTGRES_SSLMODE"),
	}
	return pgxpool.New(ctx, dsn.String())
}
