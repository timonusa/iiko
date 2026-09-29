package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

func Register(w http.ResponseWriter, r *http.Request) {

	status := http.StatusCreated
	response := ResponseT{}
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		jsonData, _ := json.Marshal(response)
		_, _ = w.Write(jsonData)
	}()

	//
	req := RegisterRequestT{}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		status = http.StatusBadRequest
		response.Error = "invalid JSON body"
		return
	}

	// ParseAddress принимает "Name <a@b>", поэтому требуем голый адрес.
	email := strings.TrimSpace(req.Email)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 {
		status = http.StatusBadRequest
		response.Error = "invalid email"
		return
	}
	email = strings.ToLower(email)

	// bcrypt учитывает только первые 72 байта пароля.
	if len(req.Password) < 8 || len(req.Password) > 72 {
		status = http.StatusBadRequest
		response.Error = "password must be 8-72 bytes long"
		return
	}

	//
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("register: hash: %v", err)
		status = http.StatusInternalServerError
		response.Error = "internal error"
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	//
	var id int64
	err = DB.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`,
		email, string(hash)).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		status = http.StatusConflict
		response.Error = "user already exists"
		return
	}
	if err != nil {
		log.Printf("register: insert user: %v", err)
		status = http.StatusServiceUnavailable
		response.Error = "service temporarily unavailable"
		return
	}

	//
	token, err := newSession(ctx, id)
	if err != nil {
		log.Printf("register: create session: %v", err)
		status = http.StatusServiceUnavailable
		response.Error = "service temporarily unavailable"
		return
	}

	//
	response.Success = true
	response.Data = RegisterResponseT{Token: token}
}
