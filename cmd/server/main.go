package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"smlt-backend/api"
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
		log.Fatalf("Failed to resolve static dir: %v", err)
	}

	fileSrv := http.FileServer(http.Dir(staticDir))

	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			handler.Handler(w, r)
			return
		}
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
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

	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
