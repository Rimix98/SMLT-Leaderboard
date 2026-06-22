package handler

import (
	"net/http"

	h "smlt-backend/handler"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	h.Handler(w, r)
}
