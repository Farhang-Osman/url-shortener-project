package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	db "github.com/Farhang-Osman/url-shortener-project/common/db"
	shortenerpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/shortenerpb"
)

const (
	kafkaBroker  = "localhost:9092"
	createdTopic = "url-created-events"
)

type server struct {
	shortenerpb.UnimplementedShortenerServiceServer
	kafkaWriter *kafka.Writer
}

type URLCreatedEvent struct {
	ShortCode string    `json:"short_code"`
	LongURL   string    `json:"long_url"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

func newServer() *server {
	// Initialize Kafka writer
	writer := &kafka.Writer{
		Addr:     kafka.TCP(kafkaBroker),
		Topic:    createdTopic,
		Balancer: &kafka.LeastBytes{},
	}

	return &server{
		kafkaWriter: writer,
	}
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

	createdAt := time.Now()
	_, err := db.DB.Exec(ctx,
		"INSERT INTO urls (short_code, long_url, user_id, expires_at, created_at) VALUES ($1, $2, $3, $4, $5)",
		shortCode, req.GetLongUrl(), userID, expiresAt, createdAt)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store URL: %v", err)
	}

	// Publish event to Kafka
	event := URLCreatedEvent{
		ShortCode: shortCode,
		LongURL:   req.GetLongUrl(),
		UserID:    req.GetUserId(),
		CreatedAt: createdAt,
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("Warning: failed to marshal URL created event: %v", err)
	} else {
		err = s.kafkaWriter.WriteMessages(ctx, kafka.Message{
			Value: eventBytes,
		})
		if err != nil {
			log.Printf("Warning: failed to publish URL created event to Kafka: %v", err)
		} else {
			log.Printf("Published URL created event to Kafka for short code: %s", shortCode)
		}
	}

	log.Printf("URL shortened successfully: %s -> %s", req.GetLongUrl(), shortCode)

	return &shortenerpb.ShortenURLResponse{
		ShortCode: shortCode,
	}, nil
}

func (s *server) GetOriginalURL(ctx context.Context, req *shortenerpb.GetOriginalURLRequest) (*shortenerpb.GetOriginalURLResponse, error) {
	log.Printf("Received GetOriginalURL request for short code: %s\n", req.GetShortCode())

	var longURL string
	var expiresAt *time.Time

	// Query database for original URL and expiration date
	row := db.DB.QueryRow(ctx, "SELECT long_url, expires_at FROM urls WHERE short_code = $1", req.GetShortCode())

	err := row.Scan(&longURL, &expiresAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "short code not found")
		}
		return nil, status.Errorf(codes.Internal, "database error: %v", err)
	}

	// Check if URL has expired
	if expiresAt != nil && time.Now().After(*expiresAt) {
		return nil, status.Errorf(codes.NotFound, "short code has expired")
	}

	log.Printf("Found original URL for %s: %s", req.GetShortCode(), longURL)

	var expiresAtStr string
	if expiresAt != nil {
		expiresAtStr = expiresAt.Format(time.RFC3339)
	}

	return &shortenerpb.GetOriginalURLResponse{
		LongUrl:   longURL,
		ExpiresAt: expiresAtStr,
	}, nil
}

func (s *server) GetURLAnalytics(ctx context.Context, req *shortenerpb.GetURLAnalyticsRequest) (*shortenerpb.GetURLAnalyticsResponse, error) {
	// 1. Fetch all analytics events
	rows, err := db.DB.Query(ctx,
		"SELECT event_type, short_code, long_url, user_id, user_agent, referer, ip_address, timestamp FROM analytics WHERE short_code = $1 ORDER BY timestamp DESC",
		req.GetShortCode())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "database query error: %v", err)
	}
	defer rows.Close()

	var analytics []*shortenerpb.AnalyticsData
	for rows.Next() {
		var eventType, shortCode string
		var timestamp time.Time
		var tempLongURL, tempUserID, tempUserAgent, tempReferer sql.NullString
		var tempIP net.IP

		if err := rows.Scan(&eventType, &shortCode, &tempLongURL, &tempUserID, &tempUserAgent, &tempReferer, &tempIP, &timestamp); err != nil {
			log.Printf("Error scanning analytics row: %v", err)
			continue
		}

		data := &shortenerpb.AnalyticsData{
			EventType: eventType,
			ShortCode: shortCode,
			Timestamp: timestamp.Format(time.RFC3339),
		}
		if tempIP != nil {
			data.IpAddress = tempIP.String()
		} else {
			data.IpAddress = "N/A"
		}

		if tempLongURL.Valid {
			data.LongUrl = tempLongURL.String
		}
		if tempUserID.Valid {
			data.UserId = tempUserID.String
		}
		if tempUserAgent.Valid {
			data.UserAgent = tempUserAgent.String
		}
		if tempReferer.Valid {
			data.Referer = tempReferer.String
		}

		analytics = append(analytics, data)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error during analytics row iteration: %v", err)
	}

	// 2. Fetch total click count
	var totalClicks int64
	err = db.DB.QueryRow(ctx, "SELECT click_count FROM urls WHERE short_code = $1", req.GetShortCode()).Scan(&totalClicks)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("Error getting click count for short code %s: %v", req.GetShortCode(), err)
	}

	return &shortenerpb.GetURLAnalyticsResponse{
		Analytics:   analytics,
		TotalClicks: totalClicks,
	}, nil
}

func main() {
	// Initialize database connection pool
	if err := db.InitDB(); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.CloseDB()

	srv := newServer()
	defer srv.kafkaWriter.Close()

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	shortenerpb.RegisterShortenerServiceServer(s, srv)

	log.Printf("Shortener Service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
