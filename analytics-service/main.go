package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/segmentio/kafka-go"
)

const (
	kakfaBroker = "localhost:9092" // Assuming Kafka is running locally
	clickTopic  = "url-click-events"
	createTopic = "url-created-events"
)

func main() {
	// Create a new Kafka reader for click events
	rClick := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{kakfaBroker},
		Topic:    clickTopic,
		GroupID:  "analytics-consumer-group",
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})

	// Create a new Kafka reader for create events (optional)
	rCreate := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{kakfaBroker},
		Topic:    createTopic,
		GroupID:  "analytics-consumer-group",
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Goroutine to consume click events
	go func() {
		for {
			m, err := rClick.ReadMessage(ctx)
			if err != nil {
				log.Printf("Error reading click message: %v", err)
				break
			}
			log.Printf("Analytics Service received click message from topic %s: %s = %s\n", m.Topic, string(m.Key), string(m.Value))
			// TODO: Process and store analytics data
		}
	}()

	// Goroutine to consume create events
	go func() {
		for {
			m, err := rCreate.ReadMessage(ctx)
			if err != nil {
				log.Printf("Error reading create message: %v", err)
				break
			}
			log.Printf("Analytics Service received create message from topic %s: %s = %s\n", m.Topic, string(m.Key), string(m.Value))
			// TODO: Process and store analytics data
		}
	}()

	log.Println("Analytics Service started. Waiting for messages...")

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println("Analytics Service shutting down...")
	rClick.Close()
	rCreate.Close()
}
