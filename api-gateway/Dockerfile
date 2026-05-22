# Use the official Go image as a base
FROM golang:1.24-alpine AS builder

# Build the api-gateway application
WORKDIR /app/api-gateway

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Use a minimal base image for the final stage
FROM alpine:latest

# Set the working directory
WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/api-gateway/main .

# Expose the port the service listens on
EXPOSE 8080

# Command to run the executable
CMD ["./main"]
