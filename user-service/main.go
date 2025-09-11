package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	db "github.com/Farhang-Osman/url-shortener-project/common/db" // Import the common db package
	userpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/userpb"
)

// Replace with your actual GitHub username
const jwtSecret = "your-secret-key-change-this-in-production"

type server struct {
	userpb.UnimplementedUserServiceServer
}

func (s *server) RegisterUser(ctx context.Context, req *userpb.RegisterUserRequest) (*userpb.RegisterUserResponse, error) {
	log.Printf("Received RegisterUser request: %v\n", req.GetUsername())

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.GetPassword()), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to hash password: %v", err)
	}

	// Insert user into database
	var userID string
	// Use db.DB from the common package
	err = db.DB.QueryRow(ctx,
		"INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3) RETURNING id",
		req.GetUsername(), req.GetEmail(), hashedPassword).Scan(&userID)

	if err != nil {
		// Check for unique constraint violation
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" { // 23505 is unique_violation
			return nil, status.Errorf(codes.AlreadyExists, "username or email already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create user: %v", err)
	}

	return &userpb.RegisterUserResponse{
		UserId:  userID,
		Message: "User registered successfully",
	}, nil
}

func (s *server) LoginUser(ctx context.Context, req *userpb.LoginUserRequest) (*userpb.LoginUserResponse, error) {
	log.Printf("Received LoginUser request: %v\n", req.GetUsername())

	// Get user from database
	var userID string
	var hashedPassword []byte // Change to []byte for bcrypt.CompareHashAndPassword
	// Use db.DB from the common package
	err := db.DB.QueryRow(ctx,
		"SELECT id, password_hash FROM users WHERE username = $1",
		req.GetUsername()).Scan(&userID, &hashedPassword)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "database error: %v", err)
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(req.GetPassword()))
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid password")
	}

	// Generate JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour * 24).Unix(), // 24 hours
	})

	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token: %v", err)
	}

	return &userpb.LoginUserResponse{
		UserId:  userID,
		Token:   tokenString,
		Message: "User logged in successfully",
	}, nil
}

func (s *server) ValidateToken(ctx context.Context, req *userpb.ValidateTokenRequest) (*userpb.ValidateTokenResponse, error) {
	log.Printf("Received ValidateToken request")

	// Parse and validate JWT token
	token, err := jwt.Parse(req.GetToken(), func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, status.Errorf(codes.Unauthenticated, "unexpected signing method")
		}
		return []byte(jwtSecret), nil
	})

	if err != nil || !token.Valid {
		return &userpb.ValidateTokenResponse{
			IsValid: false,
			UserId:  "",
		}, nil
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return &userpb.ValidateTokenResponse{
			IsValid: false,
			UserId:  "",
		}, nil
	}

	userID, ok := claims["user_id"].(string)
	if !ok {
		return &userpb.ValidateTokenResponse{
			IsValid: false,
			UserId:  "",
		}, nil
	}

	return &userpb.ValidateTokenResponse{
		IsValid: true,
		UserId:  userID,
	}, nil
}

func main() {
	// Initialize database connection pool
	if err := db.InitDB(); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.CloseDB()

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	userpb.RegisterUserServiceServer(s, &server{})

	log.Printf("User Service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
