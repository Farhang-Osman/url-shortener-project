package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	db "github.com/Farhang-Osman/url-shortener-project/common/db"
	shortenerpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/shortenerpb"
)

type server struct {
	shortenerpb.UnimplementedShortenerServiceServer
}

func (s *server) ShortenURL(ctx context.Context, req *shortenerpb.ShortenURLRequest) (*shortenerpb.ShortenURLResponse, error) {
	log.Printf("Received ShortenURL request: %v\n", req.GetLongUrl())

	// Generate short code (use custom alias if provided)
	var shortCode string
	if req.GetCustomAlias() != "" {
		shortCode = req.GetCustomAlias()

		// Check if custom alias already exists
		var exists bool
		err := db.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM urls WHERE short_code = $1)", shortCode).Scan(&exists)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "database error: %v", err)
		}
		if exists {
			return nil, status.Errorf(codes.AlreadyExists, "custom alias already exists")
		}
	} else {
		// Generate random short code and ensure it's unique
		for {
			shortCode = generateShortCode()
			var exists bool
			err := db.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM urls WHERE short_code = $1)", shortCode).Scan(&exists)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "database error: %v", err)
			}
			if !exists {
				break
			}
		}
	}

	// Parse expires_at if provided
	var expiresAt *time.Time
	if req.GetExpiresAt() != "" {
		parsedTime, err := parseExpiresAt(req.GetExpiresAt())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid expires_at format: %v", err)
		}
		expiresAt = parsedTime
	}

	// Insert URL into database
	var userID *string
	if req.GetUserId() != "" {
		userID = &req.UserId
	}

	_, err := db.DB.Exec(ctx,
		"INSERT INTO urls (short_code, long_url, user_id, expires_at, created_at) VALUES ($1, $2, $3, $4, $5)",
		shortCode, req.GetLongUrl(), userID, expiresAt, time.Now())

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store URL: %v", err)
	}

	log.Printf("URL shortened successfully: %s -> %s", req.GetLongUrl(), shortCode)

	return &shortenerpb.ShortenURLResponse{
		ShortCode: shortCode,
	}, nil
}

func (s *server) GetOriginalURL(ctx context.Context, req *shortenerpb.GetOriginalURLRequest) (*shortenerpb.GetOriginalURLResponse, error) {
	log.Printf("Received GetOriginalURL request: %v\n", req.GetShortCode())

	// Query database for the URL
	var longURL string
	var expiresAt sql.NullTime
	err := db.DB.QueryRow(ctx,
		"SELECT long_url, expires_at FROM urls WHERE short_code = $1",
		req.GetShortCode()).Scan(&longURL, &expiresAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "short URL not found")
		}
		return nil, status.Errorf(codes.Internal, "database error: %v", err)
	}

	// Check if URL has expired
	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
		return nil, status.Errorf(codes.NotFound, "short URL has expired")
	}

	// Update click count
	_, err = db.DB.Exec(ctx,
		"UPDATE urls SET click_count = click_count + 1, last_accessed = $1 WHERE short_code = $2",
		time.Now(), req.GetShortCode())
	if err != nil {
		log.Printf("Warning: failed to update click count: %v", err)
		// Don't fail the request for this
	}

	var expiresAtStr string
	if expiresAt.Valid {
		expiresAtStr = formatExpiresAt(&expiresAt.Time)
	}

	return &shortenerpb.GetOriginalURLResponse{
		LongUrl:   longURL,
		ExpiresAt: expiresAtStr,
	}, nil
}

func (s *server) UpdateURLDestination(ctx context.Context, req *shortenerpb.UpdateURLDestinationRequest) (*shortenerpb.UpdateURLDestinationResponse, error) {
	log.Printf("Received UpdateURLDestination request: %v\n", req.GetShortCode())

	// Check if the URL exists and belongs to the user
	var currentUserID sql.NullString
	err := db.DB.QueryRow(ctx,
		"SELECT user_id FROM urls WHERE short_code = $1",
		req.GetShortCode()).Scan(&currentUserID)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "short URL not found")
		}
		return nil, status.Errorf(codes.Internal, "database error: %v", err)
	}

	// Check authorization - only the owner can update
	if !currentUserID.Valid || currentUserID.String != req.GetUserId() {
		return nil, status.Errorf(codes.PermissionDenied, "you can only update your own URLs")
	}

	// Update the URL destination
	result, err := db.DB.Exec(ctx,
		"UPDATE urls SET long_url = $1, updated_at = $2 WHERE short_code = $3",
		req.GetNewLongUrl(), time.Now(), req.GetShortCode())

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update URL: %v", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, status.Errorf(codes.NotFound, "short URL not found")
	}

	return &shortenerpb.UpdateURLDestinationResponse{
		ShortCode: req.GetShortCode(),
		Message:   "URL destination updated successfully",
	}, nil
}

func main() {
	// Initialize database connection pool
	if err := db.InitDB(); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.CloseDB()

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	shortenerpb.RegisterShortenerServiceServer(s, &server{})

	log.Printf("Shortener Service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
