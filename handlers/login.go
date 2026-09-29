package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func Login(w http.ResponseWriter, r *http.Request) {

	status := http.StatusOK
	response := ResponseT{}
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		jsonData, _ := json.Marshal(response)
		_, _ = w.Write(jsonData)
	}()

	//
	req := LoginRequestT{}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		status = http.StatusBadRequest
		response.Error = "invalid JSON body"
		return
	}
	if req.Email == "" || req.Password == "" {
		status = http.StatusBadRequest
		response.Error = "email and password are required"
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	//
	email := strings.ToLower(strings.TrimSpace(req.Email))
	var id int64
	var passwordHash string
	err := DB.QueryRow(ctx, `SELECT id, password_hash FROM users WHERE email = $1`, email).
		Scan(&id, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		status = http.StatusUnauthorized
		response.Error = "invalid email or password"
		return
	}
	if err != nil {
		log.Printf("login: select user: %v", err)
		status = http.StatusServiceUnavailable
		response.Error = "service temporarily unavailable"
		return
	}

	//
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
		status = http.StatusUnauthorized
		response.Error = "invalid email or password"
		return
	}

	//
	ttl, _ := strconv.Atoi(os.Getenv("TOKEN_TTL"))
	if ttl <= 0 {
		ttl = 3600
	}

	//
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		status = http.StatusInternalServerError
		response.Error = "internal error"
		return
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))

	//
	err = Redis.Set(ctx, "session:"+hex.EncodeToString(sum[:]), id, time.Duration(ttl)*time.Second).Err()
	if err != nil {
		log.Printf("login: save session: %v", err)
		status = http.StatusServiceUnavailable
		response.Error = "service temporarily unavailable"
		return
	}

	//
	response.Success = true
	response.Data = LoginResponseT{Token: token}
}
