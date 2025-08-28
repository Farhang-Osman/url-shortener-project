# Use the official Go image as a base
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files and download dependencies
COPY go.mod go.sum ./ # Copy main go.mod/go.sum
COPY pkg/proto/go.mod pkg/proto/go.sum ./pkg/proto/
COPY shortener-service/go.mod shortener-service/go.sum ./shortener-service/

# Copy the entire project to the working directory
COPY . .

# Build the shortener-service application
WORKDIR /app/shortener-service
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Use a minimal base image for the final stage
FROM alpine:latest

# Set the working directory
WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/shortener-service/main .

# Expose the port the service listens on
EXPOSE 50052

# Command to run the executable
CMD ["./main"]
