package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

func Me(w http.ResponseWriter, r *http.Request) {

	status := http.StatusOK
	response := ResponseT{}
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		jsonData, _ := json.Marshal(response)
		_, _ = w.Write(jsonData)
	}()

	//
	scheme, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !strings.EqualFold(scheme, "Bearer") || token == "" {
		status = http.StatusUnauthorized
		response.Error = "missing or invalid authorization header"
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	// В Redis лежит хэш токена, поэтому ищем по нему.
	sum := sha256.Sum256([]byte(token))
	userID, err := Redis.Get(ctx, "session:"+hex.EncodeToString(sum[:])).Int64()
	if errors.Is(err, redis.Nil) {
		status = http.StatusUnauthorized
		response.Error = "invalid or expired token"
		return
	}
	if err != nil {
		log.Printf("me: get session: %v", err)
		status = http.StatusServiceUnavailable
		response.Error = "service temporarily unavailable"
		return
	}

	//
	var email string
	err = DB.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		status = http.StatusUnauthorized
		response.Error = "invalid or expired token"
		return
	}
	if err != nil {
		log.Printf("me: select user: %v", err)
		status = http.StatusServiceUnavailable
		response.Error = "service temporarily unavailable"
		return
	}

	//
	response.Success = true
	response.Data = MeResponseT{ID: userID, Email: email}
}
