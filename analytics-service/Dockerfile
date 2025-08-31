# Use the official Go image as a base
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files and download dependencies
COPY go.mod go.sum ./
COPY pkg/proto/go.mod pkg/proto/go.sum ./pkg/proto/
COPY analytics-service/go.mod analytics-service/go.sum ./analytics-service/

# Copy the entire project to the working directory
COPY . .

# Build the analytics-service application
WORKDIR /app/analytics-service
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Use a minimal base image for the final stage
FROM alpine:latest

# Set the working directory
WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/analytics-service/main .

# Expose the port the service listens on
# Assuming this service might expose a health check or metrics endpoint
EXPOSE 50053

# Command to run the executable
CMD ["./main"]
