package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	shortenerpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/shortenerpb"
)

const (
	shortenerServiceAddr = "localhost:50052"
	kafkaBroker          = "localhost:9092"
	clickedTopic         = "url-clicked-events"
)

type URLClickedEvent struct {
	ShortCode string    `json:"short_code"`
	ClickedAt time.Time `json:"clicked_at"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Referer   string    `json:"referer"`
}

func main() {
	// Initialize Kafka writer
	kW := &kafka.Writer{
		Addr:     kafka.TCP(kafkaBroker),
		Topic:    clickedTopic,
		Balancer: &kafka.LeastBytes{},
	}
	defer kW.Close()

	// Set up a connection to the Shortener Service
	conn, err := grpc.Dial(shortenerServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to shortener service: %v", err)
	}
	defer conn.Close()
	shortenerClient := shortenerpb.NewShortenerServiceClient(conn)

	r := mux.NewRouter()
	r.HandleFunc("/{shortCode}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		shortCode := vars["shortCode"]

		log.Printf("Received redirect request for short code: %s\n", shortCode)

		// Call Shortener Service to get original URL
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()

		res, err := shortenerClient.GetOriginalURL(ctx, &shortenerpb.GetOriginalURLRequest{
			ShortCode: shortCode,
		})
		if err != nil {
			log.Printf("Error getting original URL: %v", err)
			http.Error(w, "Short URL not found or expired", http.StatusNotFound)
			return
		}

		longURL := res.GetLongUrl()

		// Publish click event to Kafka here
		clientIP := r.RemoteAddr
		userAgent := r.Header.Get("User-Agent")
		referer := r.Header.Get("Referer")
		event := URLClickedEvent{
			ShortCode: shortCode,
			ClickedAt: time.Now(),
			IPAddress: clientIP,
			UserAgent: userAgent,
			Referer:   referer,
		}

		eventBytes, err := json.Marshal(event)
		if err != nil {
			log.Printf("Warning: failed to marshal URL clicked event: %v", err)
		} else {
			err = kW.WriteMessages(r.Context(), kafka.Message{
				Value: eventBytes,
			})
			if err != nil {
				log.Printf("Warning: failed to publish URL clicked event to Kafka: %v", err)
			} else {
				log.Printf("Published URL clicked event to Kafka for short code: %s", shortCode)
			}
		}

		log.Printf("Redirecting %s to %s\n", shortCode, longURL)
		http.Redirect(w, r, longURL, http.StatusFound)

	}).Methods("GET")

	log.Printf("Redirect Service listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", r))
}
