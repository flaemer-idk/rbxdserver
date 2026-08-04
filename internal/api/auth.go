package api

import (
	"net/http"
	"strings"
)

func AuthMiddleware(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			// Если токен не задан в конфиге — пропускаем (режим без защиты)
			next(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: missing Authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, "Unauthorized: invalid Authorization format", http.StatusUnauthorized)
			return
		}

		if parts[1] != token {
			http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}