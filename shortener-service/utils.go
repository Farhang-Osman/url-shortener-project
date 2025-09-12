package main

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"
)

// generateShortCode generates a random short code for URLs
func generateShortCode() string {
	// Generate 6 random bytes
	bytes := make([]byte, 6)
	rand.Read(bytes)

	// Encode to base64 and clean up
	encoded := base64.URLEncoding.EncodeToString(bytes)
	// Remove padding and make it URL-safe
	encoded = strings.TrimRight(encoded, "=")

	// Take first 8 characters to ensure consistent length
	if len(encoded) > 8 {
		encoded = encoded[:8]
	}

	return encoded
}

// parseExpiresAt parses ISO 8601 format string to time.Time
func parseExpiresAt(expiresAtStr string) (*time.Time, error) {
	if expiresAtStr == "" {
		return nil, nil
	}

	t, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

// formatExpiresAt formats time.Time to ISO 8601 string
func formatExpiresAt(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
