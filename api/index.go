package main

import (
	"net/http"

	"smlt-backend/handler"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	handler.Handler(w, r)
}
