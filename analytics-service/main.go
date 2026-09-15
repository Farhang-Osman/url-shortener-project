package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	db "github.com/Farhang-Osman/url-shortener-project/common/db"
	shortenerpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/shortenerpb"
)

const (
	kafkaBroker  = "localhost:9092" // Kafka broker address
	createdTopic = "url-created-events"
	clickedTopic = "url-clicked-events"
)

type analyticsServer struct {
	shortenerpb.UnimplementedShortenerServiceServer
}

type URLCreatedEvent struct {
	ShortCode string    `json:"short_code"`
	LongURL   string    `json:"long_url"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type URLClickedEvent struct {
	ShortCode string    `json:"short_code"`
	ClickedAt time.Time `json:"clicked_at"`
	UserAgent string    `json:"user_agent"`
	Referer   string    `json:"referer"`
	IPAddress string    `json:"ip_address"`
}

// GetURLAnalytics fetches all analytics data for a given short code
func (s *analyticsServer) GetURLAnalytics(ctx context.Context, req *shortenerpb.GetURLAnalyticsRequest) (*shortenerpb.GetURLAnalyticsResponse, error) {
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

	return &shortenerpb.GetURLAnalyticsResponse{
		Analytics: analytics,
	}, nil
}

func main() {
	// Initialize database connection pool
	if err := db.InitDB(); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.CloseDB()

	go func() {
		lis, err := net.Listen("tcp", ":50053")
		if err != nil {
			log.Fatalf("failed to listen on :50053: %v", err)
		}

		grpcServer := grpc.NewServer()
		shortenerpb.RegisterShortenerServiceServer(grpcServer, &analyticsServer{})
		log.Printf("Analytics gRPC service listening at %v", lis.Addr())
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("failed to serve gRPC: %v", err)
		}
	}()

	log.Println("Analytics Service started. Waiting for messages...")

	ctx := context.Background()

	// Kafka consumer for URL created events
	createdReader := kafka.NewReader(
		kafka.ReaderConfig{
			Brokers:   []string{kafkaBroker},
			Topic:     createdTopic,
			GroupID:   "analytics-created-group",
			Partition: 0,
			MinBytes:  10e3, // 10KB
			MaxBytes:  10e6, // 10MB
			MaxWait:   1 * time.Second,
			Dialer: &kafka.Dialer{
				Timeout:   10 * time.Second,
				DualStack: true,
			},
		},
	)
	defer createdReader.Close()

	// Kafka consumer for URL click events
	clickReader := kafka.NewReader(
		kafka.ReaderConfig{
			Brokers:   []string{kafkaBroker},
			Topic:     clickedTopic,
			GroupID:   "analytics-click-group",
			Partition: 0,
			MinBytes:  10e3, // 10KB
			MaxBytes:  10e6, // 10MB
			MaxWait:   1 * time.Second,
			Dialer: &kafka.Dialer{
				Timeout:   10 * time.Second,
				DualStack: true,
			},
		},
	)
	defer clickReader.Close()

	go func() {
		for {
			msg, err := createdReader.FetchMessage(ctx)
			if err != nil {
				log.Printf("Error reading create message: %v", err)
				time.Sleep(5 * time.Second) // Wait before retrying
				continue
			}

			var event URLCreatedEvent
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Printf("Error unmarshalling created event: %v", err)
				createdReader.CommitMessages(ctx, msg)
				continue
			}

			log.Printf("Received URL Created Event: ShortCode=%s, LongURL=%s", event.ShortCode, event.LongURL)

			// Store in analytics table
			_, err = db.DB.Exec(ctx,
				"INSERT INTO analytics (event_type, short_code, long_url, user_id, timestamp) VALUES ($1, $2, $3, $4, $5)",
				"url_created", event.ShortCode, event.LongURL, event.UserID, event.CreatedAt)
			if err != nil {
				log.Printf("Error storing created event in DB: %v", err)
			} else {
				log.Printf("Stored URL Created Event for short code: %s", event.ShortCode)
			}

			createdReader.CommitMessages(ctx, msg)
		}
	}()

	go func() {
		for {
			msg, err := clickReader.FetchMessage(ctx)
			if err != nil {
				log.Printf("Error reading click message: %v", err)
				time.Sleep(5 * time.Second) // Wait before retrying
				continue
			}

			var event URLClickedEvent
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Printf("Error unmarshalling click event: %v", err)
				clickReader.CommitMessages(ctx, msg)
				continue
			}

			log.Printf("Received URL Clicked Event: ShortCode=%s, IP=%s", event.ShortCode, event.IPAddress)

			// Extract IP address without port
			host, _, err := net.SplitHostPort(event.IPAddress)
			if err != nil {
				log.Printf("Warning: failed to split host and port from IPAddress %s: %v", event.IPAddress, err)
				host = event.IPAddress // Fallback to original if split fails
			}

			// Store in analytics table
			_, err = db.DB.Exec(ctx,
				"INSERT INTO analytics (event_type, short_code, user_agent, referer, ip_address, timestamp) VALUES ($1, $2, $3, $4, $5, $6)",
				"url_clicked", event.ShortCode, event.UserAgent, event.Referer, host, event.ClickedAt)
			if err != nil {
				log.Printf("Error storing click event in DB: %v", err)
			} else {
				log.Printf("Stored URL Clicked Event for short code: %s", event.ShortCode)
			}

			clickReader.CommitMessages(ctx, msg)
		}
	}()

	// Keep the main goroutine alive
	select {}
}
