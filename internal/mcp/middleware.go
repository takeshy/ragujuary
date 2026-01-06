package mcp

import (
	"net/http"
)

// APIKeyMiddleware wraps an HTTP handler with API key authentication
func APIKeyMiddleware(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check header first
		providedKey := r.Header.Get("X-API-Key")
		if providedKey == "" {
			// Fall back to Authorization header with Bearer token
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				providedKey = authHeader[7:]
			}
		}
		if providedKey == "" {
			// Fall back to query parameter
			providedKey = r.URL.Query().Get("api_key")
		}

		if providedKey != apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
