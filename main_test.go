package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewRouter проверяет, что каждая ручка доступна по своему методу и пути,
// а остальные запросы получают 405 или 404. Запросы подобраны так, чтобы хэндлер
// отвечал на этапе валидации и не обращался к БД и Redis.
func TestNewRouter(t *testing.T) {
	router := newRouter()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"register reached", http.MethodPost, "/register", "{", http.StatusBadRequest},
		{"login reached", http.MethodPost, "/login", "{", http.StatusBadRequest},
		{"me reached", http.MethodGet, "/me", "", http.StatusUnauthorized},
		{"register wrong method", http.MethodGet, "/register", "", http.StatusMethodNotAllowed},
		{"login wrong method", http.MethodGet, "/login", "", http.StatusMethodNotAllowed},
		{"me wrong method", http.MethodPost, "/me", "", http.StatusMethodNotAllowed},
		{"unknown path", http.MethodGet, "/unknown", "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}
