package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alexedwards/argon2id"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

/*
*
This struct groups the three values.
The json:"..." tags connect JSON field names to Go fields.
For example, "email" maps to Email.
*
*/
type signupRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func newSignupHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Move the existing signup handler body here.
		var input signupRequest

		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		defer r.Body.Close()

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "Invalid signup request", http.StatusBadRequest)
			return
		}

		input.Email = strings.ToLower(strings.TrimSpace(input.Email))
		input.Username = strings.TrimSpace(input.Username)

		if input.Email == "" || input.Username == "" || input.Password == "" {
			http.Error(w, "Email, username, and password are required", http.StatusBadRequest)
			return
		}

		address, err := mail.ParseAddress(input.Email)
		if err != nil || address.Address != input.Email {
			http.Error(w, "Invalid email address", http.StatusBadRequest)
			return
		}

		/**
			Username: 3–30 Unicode characters.
			Password: 15–128 Unicode characters.
		**/
		usernameLength := utf8.RuneCountInString(input.Username)
		if usernameLength < 3 || usernameLength > 30 {
			http.Error(w, "Username must be 3–30 characters", http.StatusBadRequest)
			return
		}

		passwordLength := utf8.RuneCountInString(input.Password)
		if passwordLength < 15 || passwordLength > 128 {
			http.Error(w, "Password must be 15–128 characters", http.StatusBadRequest)
			return
		}

		passwordHash, err := argon2id.CreateHash(input.Password, argon2id.DefaultParams)
		if err != nil {
			log.Print("Password hashing failed: ", err)
			http.Error(w, "Unable to create account", http.StatusInternalServerError)
			return
		}
		/**
		Save account and return its generated ID and creation time
		**/
		queryCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var userID int64
		var createdAt time.Time

		err = pool.QueryRow(queryCtx, `
			INSERT INTO users (email, username, password_hash)
			VALUES ($1, $2, $3)
			RETURNING id, created_at
		`, input.Email, input.Username, passwordHash).Scan(&userID, &createdAt)

		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) &&
				pgErr.Code == "23505" &&
				pgErr.ConstraintName == "users_email_key" {
				http.Error(w, "Email is already registered", http.StatusConflict)
				return
			}

			log.Print("User insertion failed: ", err)
			http.Error(w, "Unable to create account", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id":         userID,
			"email":      input.Email,
			"username":   input.Username,
			"created_at": createdAt,
		}); err != nil {
			log.Print("Signup response failed: ", err)
		}

	}
}
