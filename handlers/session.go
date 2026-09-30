package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"strconv"
	"time"
)

// newSession создаёт токен и сохраняет в Redis его хэш с id пользователя.
func newSession(ctx context.Context, userID int64) (string, error) {

	//
	ttl, _ := strconv.Atoi(os.Getenv("TOKEN_TTL"))
	if ttl <= 0 {
		ttl = 3600
	}

	//
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))

	//
	err := Redis.Set(ctx, "session:"+hex.EncodeToString(sum[:]), userID, time.Duration(ttl)*time.Second).Err()
	if err != nil {
		return "", err
	}
	return token, nil
}
