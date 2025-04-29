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

	socket := filepath.Join(dir, "linode.sock")
	defer os.Remove(socket)

	lis, err := net.Listen("unix", socket)
	if err != nil {
		log.Panicf("failed to listen: %v", err)
	}
	defer lis.Close()

	var opts []grpc.ServerOption

	grpcServer := grpc.NewServer(opts...)
	shared.RegisterTokenProviderServiceServer(grpcServer, &server)
	if err := grpcServer.Serve(lis); err != nil {
		log.Panicf("failed to serve: %v", err)
	}
}
