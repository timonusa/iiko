package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"service/connections"
	"service/handlers"
)

func main() {

	//
	pool, err := connections.NewPostgres(context.Background())
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	//
	rdb := connections.NewRedis()
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Printf("redis close: %v", err)
		}
	}()

	handlers.DB = pool
	handlers.Redis = rdb

	//
	server := &http.Server{
		Addr:              ":8080",
		Handler:           newRouter(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	//
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	//
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	//
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func newRouter() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", handlers.Register)
	mux.HandleFunc("POST /login", handlers.Login)
	mux.HandleFunc("GET /me", handlers.Me)
	return mux
}
