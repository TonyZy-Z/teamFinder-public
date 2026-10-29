package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

/*
*
This struct groups the two values for user input from login
*
*/
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func newLoginHandler(pool *pgxpool.Pool, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input loginRequest

		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		defer r.Body.Close()

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "Invalid login request", http.StatusBadRequest)
			return
		}

		input.Email = strings.ToLower(strings.TrimSpace(input.Email))

		if input.Email == "" || input.Password == "" {
			http.Error(w, "Email and password are required", http.StatusBadRequest)
			return
		}

		/**
		Look up the account by email
		**/
		queryCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var userID int64
		var passwordHash string

		err := pool.QueryRow(queryCtx, `
			SELECT id, password_hash
			FROM users
			WHERE email = $1
		`, input.Email).Scan(&userID, &passwordHash)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "Invalid email or password", http.StatusUnauthorized)
				return
			}

			log.Print("Login lookup failed: ", err)
			http.Error(w, "Unable to sign in", http.StatusInternalServerError)
			return
		}

		/**
		Check the submitted password against the stored hash
		**/
		match, err := argon2id.ComparePasswordAndHash(input.Password, passwordHash)
		if err != nil {
			log.Print("Password verification failed: ", err)
			http.Error(w, "Unable to sign in", http.StatusInternalServerError)
			return
		}

		if !match {
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}

		/**
		Sign tokens with JWT secret, and give tokens a 30-minute lifetime
		**/
		now := time.Now()
		expiresAt := now.Add(30 * time.Minute)

		/**
		Token's content
		**/
		claims := jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			Issuer:    "teamfinder",
			Audience:  jwt.ClaimStrings{"teamfinder-app"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		}

		/**
		Choose signing algorithm and token's content
		**/
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

		/**
		Sign
		**/
		accessToken, err := token.SignedString([]byte(jwtSecret))
		if err != nil {
			log.Print("Token signing failed: ", err)
			http.Error(w, "Unable to sign in", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"access_token": accessToken,
			"token_type":   "Bearer",
			"expires_in":   1800,
		}); err != nil {
			log.Print("Login response failed: ", err)
		}
	}
}
