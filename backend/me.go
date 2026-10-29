package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/TonyZy-Z/teamFinder/backend/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newMeHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Move your existing meHandler body here.
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		/**
		Profile look up
		**/
		queryCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var email string
		var username string
		var createdAt time.Time

		err := pool.QueryRow(queryCtx, `
			SELECT email, username, created_at
			FROM users
			WHERE id = $1
		`, userID).Scan(&email, &username, &createdAt)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			log.Print("Profile lookup failed: ", err)
			http.Error(w, "Unable to retrieve profile", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(map[string]any{
			"id":         userID,
			"email":      email,
			"username":   username,
			"created_at": createdAt,
		}); err != nil {
			log.Print("User response failed: ", err)
		}

	}
}
