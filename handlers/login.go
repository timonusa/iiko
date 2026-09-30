package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// Login обрабатывает POST /login: проверяет email и пароль и создаёт новую сессию.
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
	token, err := newSession(ctx, id)
	if err != nil {
		log.Printf("login: create session: %v", err)
		status = http.StatusServiceUnavailable
		response.Error = "service temporarily unavailable"
		return
	}

	//
	response.Success = true
	response.Data = LoginResponseT{Token: token}
}
