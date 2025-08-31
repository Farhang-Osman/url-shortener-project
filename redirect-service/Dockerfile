# Use the official Go image as a base
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files and download dependencies
COPY go.mod go.sum ./
COPY pkg/proto/go.mod pkg/proto/go.sum ./pkg/proto/
COPY redirect-service/go.mod redirect-service/go.sum ./redirect-service/

# Copy the entire project to the working directory
COPY . .

# Build the redirect-service application
WORKDIR /app/redirect-service
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Use a minimal base image for the final stage
FROM alpine:latest

# Set the working directory
WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/redirect-service/main .

# Expose the port the service listens on
EXPOSE 8080

# Command to run the executable
CMD ["./main"]
