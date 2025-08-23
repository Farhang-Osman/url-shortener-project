package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	shortenerpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/shortenerpb" // IMPORTANT: Use your main module path
)

// server is used to implement shortenerpb.ShortenerServiceServer.
type server struct {
	shortenerpb.UnimplementedShortenerServiceServer
	// In a real application, this would be a database client
	urlStore map[string]*shortenerpb.ShortenURLRequest // Dummy store
}

func newServer() *server {
	return &server{
		urlStore: make(map[string]*shortenerpb.ShortenURLRequest),
	}
}

// ShortenURL implements shortenerpb.ShortenerServiceServer
func (s *server) ShortenURL(ctx context.Context, req *shortenerpb.ShortenURLRequest) (*shortenerpb.ShortenURLResponse, error) {
	log.Printf("Received ShortenURL request: %v\n", req.GetLongUrl())

	shortCode := req.GetCustomAlias()
	if shortCode == "" {
		// TODO: Implement actual short code generation logic (e.g., base62 encoding of a unique ID)
		shortCode = fmt.Sprintf("short-%d", time.Now().UnixNano())
	}

	if _, exists := s.urlStore[shortCode]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "short code %s already exists", shortCode)
	}

	s.urlStore[shortCode] = req // Store the request for later lookup

	log.Printf("Shortened %s to %s\n", req.GetLongUrl(), shortCode)
	return &shortenerpb.ShortenURLResponse{
		ShortCode: shortCode,
	}, nil
}

// GetOriginalURL implements shortenerpb.ShortenerServiceServer
func (s *server) GetOriginalURL(ctx context.Context, req *shortenerpb.GetOriginalURLRequest) (*shortenerpb.GetOriginalURLResponse, error) {
	log.Printf("Received GetOriginalURL request: %v\n", req.GetShortCode())

	storedReq, ok := s.urlStore[req.GetShortCode()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "short code %s not found", req.GetShortCode())
	}

	// Check for expiration
	if storedReq.GetExpiresAt() != "" {
		expiresAt, err := time.Parse(time.RFC3339, storedReq.GetExpiresAt())
		if err != nil {
			log.Printf("Error parsing expires_at: %v", err)
			// Treat as expired if parsing fails for simplicity in dummy
			return nil, status.Errorf(codes.Internal, "invalid expiration format")
		}
		if time.Now().After(expiresAt) {
			return nil, status.Errorf(codes.NotFound, "short code %s has expired", req.GetShortCode())
		}
	}

	log.Printf("Found original URL for %s: %s\n", req.GetShortCode(), storedReq.GetLongUrl())
	return &shortenerpb.GetOriginalURLResponse{
		LongUrl:   storedReq.GetLongUrl(),
		ExpiresAt: storedReq.GetExpiresAt(),
	}, nil
}

// UpdateURLDestination implements shortenerpb.ShortenerServiceServer
func (s *server) UpdateURLDestination(ctx context.Context, req *shortenerpb.UpdateURLDestinationRequest) (*shortenerpb.UpdateURLDestinationResponse, error) {
	log.Printf("Received UpdateURLDestination request for %v\n", req.GetShortCode())

	storedReq, ok := s.urlStore[req.GetShortCode()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "short code %s not found", req.GetShortCode())
	}

	// TODO: Implement actual user_id authorization check against storedReq.GetUserId()
	// For this dummy, we'll assume the user_id is correct or not checked.

	storedReq.LongUrl = req.GetNewLongUrl()
	s.urlStore[req.GetShortCode()] = storedReq // Update the dummy store

	log.Printf("Updated %s to new URL %s\n", req.GetShortCode(), req.GetNewLongUrl())
	return &shortenerpb.UpdateURLDestinationResponse{
		ShortCode: req.GetShortCode(),
		Message:   "URL updated successfully (dummy)",
	}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50052") // Listen on port 50052
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	shortenerpb.RegisterShortenerServiceServer(s, newServer())

	log.Printf("Shortener Service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
