package httpresponse

import (
	"encoding/json"
	"net/http"
)

func WriteError(responseWriter http.ResponseWriter, status int, message string) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(status)
	json.NewEncoder(responseWriter).Encode(map[string]string{"error": message})
}
