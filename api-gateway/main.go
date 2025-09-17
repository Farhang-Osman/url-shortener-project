package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	shortenerpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/shortenerpb"
	userpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/userpb"
)

const ( // gRPC service addresses
	userServiceAddress      = "localhost:50051"
	shortenerServiceAddress = "localhost:50052"
	jwtSecret               = "your-secret-key-change-this-in-production" // Must match User Service
)

type APIGateway struct {
	userClient      userpb.UserServiceClient
	shortenerClient shortenerpb.ShortenerServiceClient
}

func NewAPIGateway(userConn *grpc.ClientConn, shortenerConn *grpc.ClientConn) *APIGateway {
	return &APIGateway{
		userClient:      userpb.NewUserServiceClient(userConn),
		shortenerClient: shortenerpb.NewShortenerServiceClient(shortenerConn),
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

	apig := NewAPIGateway(userConn, shortenerConn)

	r := mux.NewRouter()

	// Public routes
	r.HandleFunc("/register", apig.RegisterUser).Methods("POST")
	r.HandleFunc("/login", apig.LoginUser).Methods("POST")

	// Authenticated routes - using individual middleware wrapping instead of subrouter
	r.Handle("/auth/shorten", apig.AuthMiddleware(http.HandlerFunc(apig.ShortenURL))).Methods("POST")
	r.Handle("/auth/update/{shortCode}", apig.AuthMiddleware(http.HandlerFunc(apig.UpdateURLDestination))).Methods("PUT")

	log.Printf("API Gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
