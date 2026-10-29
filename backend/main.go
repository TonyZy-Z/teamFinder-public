package main

import (
	"context"
	"errors"
	"github.com/TonyZy-Z/teamFinder/backend/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	/**
	Read .env from directory
	**/
	if err := godotenv.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatal("Unable to load .env file")
		}
	}

	/**
	Read and validate JWT_SECRET from .env
	**/
	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 64 {
		log.Fatal("JWT_SECRET must contain at least 64 characters")
	}

	// database set up
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	/**
	Connection pool setup
	**/
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal("Database pool setup failed: ", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal("Database ping failed: ", err)
	}

	log.Println("Connected to PostgreSQL")

	/**
	Redis client startup begins
	**/
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("REDIS_ADDR is not set")
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer redisClient.Close()

	redisCtx, cancelRedis := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRedis()

	if err := redisClient.Ping(redisCtx).Err(); err != nil {
		log.Fatal("Redis connection failed: ", err)
	}

	log.Println("Connected to Redis")
	/**
	Redis client startup ends
	**/

	/**
	Handlers registration begins
	**/

	// http server setup
	mux := http.NewServeMux()

	/**
	Register protected http handler behind the middleware
	First of its kind: /auth/me
	**/
	requireAuth := auth.RequireAuth(jwtSecret)
	mux.Handle("GET /auth/me", requireAuth(newMeHandler(pool)))

	/**
	Login handler
	**/
	mux.HandleFunc("POST /auth/login", newLoginHandler(pool, jwtSecret))

	/**
	Signup handler
	**/
	mux.HandleFunc("POST /auth/signup", newSignupHandler(pool))

	/**
	Heath handler
	**/
	mux.HandleFunc("GET /health", healthHandler)

	/**
	Games handler
	**/
	mux.HandleFunc("GET /games", gamesHandler)

	/**
	Matchmaking handler: join
	**/
	mux.Handle(
		"POST /matchmaking/queue",
		requireAuth(newJoinQueueHandler(redisClient)),
	)

	/**
	Matchmaking handler: cancel
	**/
	mux.Handle(
		"DELETE /matchmaking/queue",
		requireAuth(newCancelQueueHandler(redisClient)),
	)

	/**
	Get state handler
	**/
	mux.Handle(
		"GET /matchmaking/status",
		requireAuth(newMatchmakingStatusHandler(redisClient)),
	)

	/**
	Handlers registration ends
	**/

	log.Println("Server listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
