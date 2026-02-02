module github.com/Farhang-Osman/url-shortener-project/api-gateway

go 1.24.6

require (
	github.com/Farhang-Osman/url-shortener-project/pkg/proto v0.0.0-20250822173454-061879e34199
	github.com/gorilla/mux v1.8.1
	google.golang.org/grpc v1.75.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/redis/go-redis/v9 v9.17.3 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)

replace github.com/Farhang-Osman/url-shortener-project/pkg/proto => ../pkg/proto
