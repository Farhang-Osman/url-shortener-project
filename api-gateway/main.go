package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	shortenerpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/shortenerpb" // IMPORTANT: Use your main module path
	userpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/userpb"           // IMPORTANT: Use your main module path
)

const (
	userServiceAddr      = "localhost:50051"
	shortenerServiceAddr = "localhost:50052"
)

type apiGateway struct {
	shortenerClient shortenerpb.ShortenerServiceClient
	userClient      userpb.UserServiceClient
}

func newAPIGateway(shortenerConn *grpc.ClientConn, userConn *grpc.ClientConn) *apiGateway {
	return &apiGateway{
		shortenerClient: shortenerpb.NewShortenerServiceClient(shortenerConn),
		userClient:      userpb.NewUserServiceClient(userConn),
	}
}

type ShortenRequest struct {
	LongURL     string `json:"long_url"`
	CustomAlias string `json:"custom_alias,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

type UpdateRequest struct {
	NewLongURL string `json:"new_long_url"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// authMiddleware validates JWT token and adds user_id to context
func (a *apiGateway) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()

			resp, err := a.userClient.ValidateToken(ctx, &userpb.ValidateTokenRequest{Token: token})
			if err != nil || !resp.GetIsValid() {
				log.Printf("Token validation failed: %v", err)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Add user_id to request context for downstream handlers
			ctx = context.WithValue(r.Context(), "user_id", resp.GetUserId())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
}

func (a *apiGateway) shortenURLHandler(w http.ResponseWriter, r *http.Request) {
	var req ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID, _ := r.Context().Value("user_id").(string) // user_id might be empty if not authenticated

	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	grpcReq := &shortenerpb.ShortenURLRequest{
		LongUrl:     req.LongURL,
		CustomAlias: req.CustomAlias,
		ExpiresAt:   req.ExpiresAt,
		UserId:      userID,
	}

	res, err := a.shortenerClient.ShortenURL(ctx, grpcReq)
	if err != nil {
		s, ok := status.FromError(err)
		if ok && s.Code() == codes.AlreadyExists {
			http.Error(w, s.Message(), http.StatusConflict)
			return
		}
		log.Printf("Error from Shortener Service: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"short_url": "http://localhost:8080/" + res.GetShortCode()})
}

func (a *apiGateway) updateURLDestinationHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shortCode := vars["shortCode"]

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		http.Error(w, "User ID not found in context", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	grpcReq := &shortenerpb.UpdateURLDestinationRequest{
		ShortCode:  shortCode,
		NewLongUrl: req.NewLongURL,
		UserId:     userID,
	}

	res, err := a.shortenerClient.UpdateURLDestination(ctx, grpcReq)
	if err != nil {
		s, ok := status.FromError(err)
		if ok && s.Code() == codes.NotFound {
			http.Error(w, s.Message(), http.StatusNotFound)
			return
		}
		if ok && s.Code() == codes.PermissionDenied {
			// This is a dummy check, in real implementation, Shortener Service would return this
			http.Error(w, s.Message(), http.StatusForbidden)
			return
		}
		log.Printf("Error from Shortener Service: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"short_code": res.GetShortCode(), "message": res.GetMessage()})
}

func (a *apiGateway) registerUserHandler(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	grpcReq := &userpb.RegisterUserRequest{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	}

	res, err := a.userClient.RegisterUser(ctx, grpcReq)
	if err != nil {
		log.Printf("Error from User Service (RegisterUser): %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"user_id": res.GetUserId(), "message": res.GetMessage()})
}

func (a *apiGateway) loginUserHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	grpcReq := &userpb.LoginUserRequest{
		Username: req.Username,
		Password: req.Password,
	}

	res, err := a.userClient.LoginUser(ctx, grpcReq)
	if err != nil {
		log.Printf("Error from User Service (LoginUser): %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"user_id": res.GetUserId(), "token": res.GetToken(), "message": res.GetMessage()})
}

func main() {
	// Set up connections to gRPC services
	userConn, err := grpc.Dial(userServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to user service: %v", err)
	}
	defer userConn.Close()

	shortenerConn, err := grpc.Dial(shortenerServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to shortener service: %v", err)
	}
	defer shortenerConn.Close()

	api := newAPIGateway(shortenerConn, userConn)

	r := mux.NewRouter()

	// Public routes
	r.HandleFunc("/register", api.registerUserHandler).Methods("POST")
	r.HandleFunc("/login", api.loginUserHandler).Methods("POST")

	// Authenticated routes
	authRouter := r.PathPrefix("").Subrouter()
	authRouter.Use(api.authMiddleware)
	authRouter.HandleFunc("/shorten", api.shortenURLHandler).Methods("POST")
	authRouter.HandleFunc("/shorten/{shortCode}", api.updateURLDestinationHandler).Methods("PUT")

	log.Printf("API Gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
