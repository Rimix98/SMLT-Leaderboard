package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"smlt-backend/handler"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	staticDir := "Frontend/dist"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		staticDir = "../Frontend/dist"
	}

	absStaticDir, err := filepath.Abs(staticDir)
	if err != nil {
		slog.Error("failed to resolve static dir", "error", err)
		os.Exit(1)
	}

	fileSrv := http.FileServer(http.Dir(staticDir))

	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			handler.Handler(w, r)
			return
		}

		cleaned := filepath.Clean(r.URL.Path)
		resolved := filepath.Join(staticDir, cleaned)
		absResolved, err := filepath.Abs(resolved)
		if err != nil || !strings.HasPrefix(absResolved, absStaticDir) {
			http.NotFound(w, r)
			return
		}

		if _, err := os.Stat(absResolved); err == nil {
			fileSrv.ServeHTTP(w, r)
			return
		}

		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced shutdown", "error", err)
	}
	slog.Info("server stopped")
}
