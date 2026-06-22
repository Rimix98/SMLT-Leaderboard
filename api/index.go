package handler

import (
	"net/http"

	handlerpkg "smlt-backend/handler"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	handlerpkg.Handler(w, r)
}
