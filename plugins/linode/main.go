package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/guilhem/token-renewer/shared"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	pluginName    = "linode"
	pluginVersion = "v0.1.0"
)

func main() {
	var socketPath string

	flag.StringVar(&socketPath, "socket-path", "/plugins/linode.sock", "Path to the Unix socket")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))
	log.SetLogger(logger)

	setupLog := logger.WithName("setup")

	setupLog.Info("Starting Linode plugin",
		"name", pluginName,
		"version", pluginVersion,
		"socket", socketPath,
	)

	// Handle graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create Linode plugin instance
	linodePlugin := &LinodePlugin{}

	// Start gRPC server on Unix socket
	if err := runPlugin(ctx, socketPath, linodePlugin); err != nil {
		if err != context.Canceled {
			setupLog.Error(err, "plugin failed")
			os.Exit(1)
		}
	}

	setupLog.Info("Plugin stopped gracefully")
}

// runPlugin starts a gRPC server that implements TokenProviderService on a Unix socket.
func runPlugin(ctx context.Context, socketPath string, plugin *LinodePlugin) error {
	// Listen on Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %s: %w", socketPath, err)
	}
	defer listener.Close()

	log.Log.Info("Plugin listening on socket", "path", socketPath)

	// Create gRPC server
	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	// Register the plugin as the implementation of TokenProviderService
	shared.RegisterTokenProviderServiceServer(grpcServer, plugin)

	log.Log.Info("Serving gRPC requests", "name", pluginName)

	// Run server in a goroutine
	serverErrs := make(chan error, 1)
	go func() {
		serverErrs <- grpcServer.Serve(listener)
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Log.Info("Shutting down gRPC server")
		grpcServer.GracefulStop()
		return nil
	case err := <-serverErrs:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
}
