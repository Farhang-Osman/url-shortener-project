package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userpb "github.com/Farhang-Osman/url-shortener-project/pkg/proto/userpb" // IMPORTANT: Use your main module path
)

// server is used to implement userpb.UserServiceServer.
type server struct {
	userpb.UnimplementedUserServiceServer
}

// RegisterUser implements userpb.UserServiceServer
func (s *server) RegisterUser(ctx context.Context, req *userpb.RegisterUserRequest) (*userpb.RegisterUserResponse, error) {
	log.Printf("Received RegisterUser request: %v\n", req.GetUsername())
	// TODO: Implement user registration logic (e.g., hash password, save to DB)
	// For now, just return a dummy response
	return &userpb.RegisterUserResponse{
		UserId:  "user-" + req.GetUsername(),
		Message: "User registered successfully (dummy)",
	}, nil
}

// LoginUser implements userpb.UserServiceServer
func (s *server) LoginUser(ctx context.Context, req *userpb.LoginUserRequest) (*userpb.LoginUserResponse, error) {
	log.Printf("Received LoginUser request: %v\n", req.GetUsername())
	// TODO: Implement user login logic (e.g., verify password, generate JWT)
	// For now, just return a dummy response
	return &userpb.LoginUserResponse{
		UserId:  "user-" + req.GetUsername(),
		Token:   "dummy-jwt-token",
		Message: "User logged in successfully (dummy)",
	}, nil
}

// ValidateToken implements userpb.UserServiceServer
func (s *server) ValidateToken(ctx context.Context, req *userpb.ValidateTokenRequest) (*userpb.ValidateTokenResponse, error) {
	log.Printf("Received ValidateToken request: %v\n", req.GetToken())
	// TODO: Implement token validation logic
	// For now, just return a dummy response
	if req.GetToken() == "dummy-jwt-token" {
		return &userpb.ValidateTokenResponse{
			IsVaild: true,
			UserId:  "user-dummy",
		}, nil
	}
	return &userpb.ValidateTokenResponse{
		IsVaild: false,
		UserId:  "",
	}, status.Errorf(codes.Unauthenticated, "invalid token")
}

func main() {
	lis, err := net.Listen("tcp", ":50051") // Listen on port 50051
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	userpb.RegisterUserServiceServer(s, &server{})

	log.Printf("User Service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
