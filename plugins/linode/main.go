package main

import (
	"log"
	"net"
	"os"
	"path/filepath"

	"github.com/guilhem/token-renewer/shared"
	"google.golang.org/grpc"
)

func main() {
	linodePlugin := &LinodePlugin{}

	server := shared.GRPCServer{
		Impl: linodePlugin,
	}

	dir, ok := os.LookupEnv("PLUGIN_DIR")
	if !ok {
		dir = "/plugins"
	}

	lis, err := net.Listen("unix", filepath.Join(dir, "linode.sock"))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption

	grpcServer := grpc.NewServer(opts...)
	shared.RegisterTokenProviderServiceServer(grpcServer, &server)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
