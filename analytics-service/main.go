package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/segmentio/kafka-go"

	db "github.com/Farhang-Osman/url-shortener-project/common/db"
)

const (
	kafkaBroker  = "localhost:9092" // Kafka broker address
	createdTopic = "url-created-events"
	clickTopic   = "url-click-events"
)

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

func main() {
	// Initialize database connection pool
	if err := db.InitDB(); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.CloseDB()

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
			Topic:     clickTopic,
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

			// Store in analytics table
			_, err = db.DB.Exec(ctx,
				"INSERT INTO analytics (event_type, short_code, user_agent, referer, ip_address, timestamp) VALUES ($1, $2, $3, $4, $5, $6)",
				"url_clicked", event.ShortCode, event.UserAgent, event.Referer, event.IPAddress, event.ClickedAt)
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
