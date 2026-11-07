package main

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TonyZy-Z/teamFinder/backend/internal/auth"
	"github.com/TonyZy-Z/teamFinder/backend/internal/matchmaking"
	"github.com/TonyZy-Z/teamFinder/backend/internal/realtime"
	lksdk "github.com/livekit/server-sdk-go/v2"
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

	// Load the LiveKit project settings from the environment
	livekitURL := os.Getenv("LIVEKIT_URL")
	livekitAPIKey := os.Getenv("LIVEKIT_API_KEY")
	livekitAPISecret := os.Getenv("LIVEKIT_API_SECRET")

	if livekitURL == "" || livekitAPIKey == "" || livekitAPISecret == "" {
		log.Fatal("LIVEKIT_URL, LIVEKIT_API_KEY, and LIVEKIT_API_SECRET are required")
	}

	log.Println("LiveKit configuration loaded")

	// Use our credentials for LiveKit room management requests
	roomClient := lksdk.NewRoomServiceClient(
		livekitURL,
		livekitAPIKey,
		livekitAPISecret,
	)

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

	// Ctrl+C or SIGTERM request a clean shutdown
	workerCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// Share one connection registry across notification handlers
	connections := realtime.NewConnections()

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
		requireAuth(newJoinQueueHandler(redisClient, connections)),
	)

	/**
	Matchmaking handler: cancel
	**/
	mux.Handle(
		"DELETE /matchmaking/queue",
		requireAuth(newCancelQueueHandler(redisClient, connections)),
	)

	/**
	Get state handler
	**/
	mux.Handle(
		"GET /matchmaking/status",
		requireAuth(newMatchmakingStatusHandler(redisClient)),
	)

	/**
	Confirm match handler
	**/
	mux.Handle(
		"POST /matchmaking/matches/{match_id}/confirm",
		requireAuth(newConfirmMatchHandler(redisClient, connections)),
	)

	/**
	WebSocket handler
	**/
	mux.Handle(
		"GET /ws",
		requireAuth(newWebSocketHandler(workerCtx, connections, redisClient)),
	)

	/**
	Voice room token request handler
	**/
	mux.Handle(
		"POST /voice/token",
		requireAuth(newVoiceTokenHandler(
			redisClient,
			livekitURL,
			livekitAPIKey,
			livekitAPISecret,
		)),
	)

	/**
	Leave voice room handler
	**/
	mux.Handle(
		"POST /matchmaking/matches/{match_id}/leave",
		requireAuth(newLeaveMatchHandler(
			redisClient,
			roomClient,
			connections,
		)),
	)

	/**
	Handlers registration ends
	**/

	// Start timeout processing and record when it finished
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		matchmaking.RunTimeoutWorker(
			workerCtx,
			redisClient,
			func(ctx context.Context, matchID string) {
				// The expired hash retains participants IDs for notification
				notifyMatchParticipants(ctx, redisClient, connections, matchID)
			},
		)
	}()

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Run the server while main waits for shutdown or a server error
	serverErrors := make(chan error, 1)
	go func() {
		log.Println("Server listening on http://localhost:8080")
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-workerCtx.Done():
		log.Println("Stopping backend")
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server failed: %v", err)
		}
	}

	// Stop the worker and allow HTTP requests time to finish
	stop()

	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown failed: %v", err)
		if err := server.Close(); err != nil {
			log.Printf("HTTP close failed: %v", err)
		}
	}

	// Wait before main returns and its deferred Redis close runs
	<-workerDone
}
