package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	shortenerpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/shortenerpb"
	userpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/userpb"
)

const ( // gRPC service addresses
	userServiceAddress      = "localhost:50051"
	shortenerServiceAddress = "localhost:50052"
	redisClientAddress      = "localhost:6379"
	// jwtSecret               = "your-secret-key-change-this-in-production" // Must match User Service
)

type APIGateway struct {
	userClient      userpb.UserServiceClient
	shortenerClient shortenerpb.ShortenerServiceClient
	redisClient     *redis.Client
}

func NewAPIGateway(userConn *grpc.ClientConn, shortenerConn *grpc.ClientConn, redisClient *redis.Client) *APIGateway {
	return &APIGateway{
		userClient:      userpb.NewUserServiceClient(userConn),
		shortenerClient: shortenerpb.NewShortenerServiceClient(shortenerConn),
		redisClient:     redisClient,
	}
}

// AuthMiddleware validates JWT token and sets user_id in context
func (g *APIGateway) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Authorization header required"})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Bearer token required"})
			return
		}

		// Validate token with User Service
		res, err := g.userClient.ValidateToken(r.Context(), &userpb.ValidateTokenRequest{Token: tokenString})
		if err != nil || !res.GetIsValid() {
			log.Printf("Token validation failed: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid or expired token"})
			return
		}

		// Set user_id in context for downstream handlers
		ctx := context.WithValue(r.Context(), "userID", res.GetUserId())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RegisterUser handles user registration
func (g *APIGateway) RegisterUser(w http.ResponseWriter, r *http.Request) {
	var req userpb.RegisterUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	res, err := g.userClient.RegisterUser(r.Context(), &req)
	if err != nil {
		log.Printf("Error from User Service (RegisterUser): %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("User registration failed: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"user_id": res.GetUserId(), "message": res.GetMessage()})
}

// LoginUser handles user login
func (g *APIGateway) LoginUser(w http.ResponseWriter, r *http.Request) {
	var req userpb.LoginUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	res, err := g.userClient.LoginUser(r.Context(), &req)
	if err != nil {
		log.Printf("Error from User Service (LoginUser): %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("User login failed: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"user_id": res.GetUserId(), "token": res.GetToken(), "message": res.GetMessage()})
}

// ShortenURL handles URL shortening
func (g *APIGateway) ShortenURL(w http.ResponseWriter, r *http.Request) {
	var req shortenerpb.ShortenURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Get user_id from context (set by AuthMiddleware)
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "User not authenticated"})
		return
	}
	req.UserId = userID

	res, err := g.shortenerClient.ShortenURL(r.Context(), &req)
	if err != nil {
		log.Printf("Error from Shortener Service (ShortenURL): %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("URL shortening failed: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"short_code": res.GetShortCode()})
}

// UpdateURLDestination handles updating a short URL's destination
func (g *APIGateway) UpdateURLDestination(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shortCode := vars["shortCode"]

	var req shortenerpb.UpdateURLDestinationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	req.ShortCode = shortCode // Set shortCode from URL path

	// Get user_id from context (set by AuthMiddleware)
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "User not authenticated"})
		return
	}
	req.UserId = userID

	res, err := g.shortenerClient.UpdateURLDestination(r.Context(), &req)
	if err != nil {
		log.Printf("Error from Shortener Service (UpdateURLDestination): %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("URL update failed: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"short_code": res.GetShortCode(), "message": res.GetMessage()})
}

// GetURLAnalytics handles fetching analytics data for a short URL
func (g *APIGateway) GetURLAnalytics(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shortCode := vars["shortCode"]

	// Get user_id from context (set by AuthMiddleware)
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "User not authenticated"})
		return
	}

	log.Printf("Received analytics request for short code: %s by user: %s\n", shortCode, userID)

	ctx, cancel := context.WithTimeout(r.Context(), time.Second*5)
	defer cancel()

	res, err := g.shortenerClient.GetURLAnalytics(ctx, &shortenerpb.GetURLAnalyticsRequest{
		ShortCode: shortCode,
		UserId:    userID, // Pass user ID for authorization check in Shortener Service
	})
	if err != nil {
		log.Printf("Error getting URL analytics: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to retrieve analytics: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// RateLimitMiddleware limits requests to 'limit' per 'window' duration
func (g *APIGateway) RateLimitMiddleware(limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Safety check for nil redis client
			if g.redisClient == nil {
				log.Println("Error: redisClient is nil in RateLimitMiddleware")
				next.ServeHTTP(w, r)
				return
			}

			// Identify the user. Use IP without port for consistency.
			identifier, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				identifier = r.RemoteAddr // Fallback if SplitHostPort fails
			}

			// Override with userID if authenticated
			if userID, ok := r.Context().Value("userID").(string); ok && userID != "" {
				identifier = userID
			}

			key := fmt.Sprintf("rate_limit:%s", identifier)
			ctx := r.Context()

			// Increment the counter
			count, err := g.redisClient.Incr(ctx, key).Result()
			if err != nil {
				log.Printf("Redis error in rate limiter: %v", err)
				next.ServeHTTP(w, r) // Fail open to avoid blocking users on Redis failure
				return
			}

			if count == 1 {
				g.redisClient.Expire(ctx, key, window)
			}

			if count > int64(limit) {
				log.Printf("Rate limit exceeded for %s (count: %d)", identifier, count)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "Rate limit exceeded. Please try again later.",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	// Set up gRPC connections
	userConn, err := grpc.Dial(userServiceAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to user service: %v", err)
	}
	defer userConn.Close()

	shortenerConn, err := grpc.Dial(shortenerServiceAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to shortener service: %v", err)
	}
	defer shortenerConn.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: redisClientAddress,
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	log.Println("Connected to Redis successfully")

	apig := NewAPIGateway(userConn, shortenerConn, rdb)

	r := mux.NewRouter()

	publicLimit := 3 // 3 requests per minute for public routes
	authLimit := 6   // 6 requests per minute for authenticated users
	window := time.Minute

	// Public routes with rate limiting
	r.Handle("/register", apig.RateLimitMiddleware(publicLimit, window)(http.HandlerFunc(apig.RegisterUser))).Methods("POST")
	r.Handle("/login", apig.RateLimitMiddleware(publicLimit, window)(http.HandlerFunc(apig.LoginUser))).Methods("POST")

	// AuthMiddleware FIRST so the RateLimiter can use the userID
	// Shorten URL
	shortenHandler := apig.AuthMiddleware(http.HandlerFunc(apig.ShortenURL))
	r.Handle("/auth/shorten", apig.RateLimitMiddleware(authLimit, window)(shortenHandler)).Methods("POST")

	// Update URL
	updateHandler := apig.AuthMiddleware(http.HandlerFunc(apig.UpdateURLDestination))
	r.Handle("/auth/update/{shortCode}", apig.RateLimitMiddleware(authLimit, window)(updateHandler)).Methods("PUT")

	// Get Analytics
	analyticsHandler := apig.AuthMiddleware(http.HandlerFunc(apig.GetURLAnalytics))
	r.Handle("/auth/analytics/{shortCode}", apig.RateLimitMiddleware(authLimit, window)(analyticsHandler)).Methods("GET")

	log.Printf("API Gateway listening on :8080 with Rate Limiting enabled")
	log.Fatal(http.ListenAndServe(":8080", r))
}
