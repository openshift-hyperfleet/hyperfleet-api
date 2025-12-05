package handlers

import (
	"encoding/json"
	"net/http"
)

func writeJSONResponse(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	// By default, decide whether or not a cache is usable based on the matching of the JWT
	// For example, this will keep caches from being used in the same browser if two users were to log in back to back
	w.Header().Set("Vary", "Authorization")

	w.WriteHeader(code)

	if payload != nil {
		response, err := json.Marshal(payload)
		if err != nil {
			// Headers already sent, can't change status code
			return
		}
		if _, err := w.Write(response); err != nil {
			// Writing failed, nothing we can do at this point
			return
		}
	}
}

// Prepare a 'list' of non-db-backed resources
