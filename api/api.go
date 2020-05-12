/*
Package api defines the Radish gRPC service.
*/
package api

//go:generate protoc -I . --go_out=plugins=grpc:. radish.proto
