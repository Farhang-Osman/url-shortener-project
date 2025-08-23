package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	shortenerpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/shortenerpb" // IMPORTANT: Use your main module path
)

const (
	shortenerServiceAddr = "localhost:50052"
)

func main() {
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
		expiresAt := res.GetExpiresAt()

		// Check for expiration (redundant with Shortener Service, but good for robustness)
		if expiresAt != "" {
			exp, parseErr := time.Parse(time.RFC3339, expiresAt)
			if parseErr == nil && time.Now().After(exp) {
				http.Error(w, "Short URL has expired", http.StatusNotFound)
				return
			}
		}

		// TODO: Publish click event to Kafka here
		log.Printf("Redirecting %s to %s\n", shortCode, longURL)
		http.Redirect(w, r, longURL, http.StatusFound)

	}).Methods("GET")

	log.Printf("Redirect Service listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
