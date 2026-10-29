package auth

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

func RequireAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			parts := strings.Fields(r.Header.Get("Authorization"))
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			claims := &jwt.RegisteredClaims{}

			token, err := jwt.ParseWithClaims(
				parts[1],
				claims,
				func(token *jwt.Token) (any, error) {
					return []byte(secret), nil
				},
				jwt.WithValidMethods([]string{"HS256"}),
				jwt.WithExpirationRequired(),
				jwt.WithIssuer("teamfinder"),
				jwt.WithAudience("teamfinder-app"),
			)
			if err != nil || !token.Valid {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			userID, err := strconv.ParseInt(claims.Subject, 10, 64)
			if err != nil || userID <= 0 {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}