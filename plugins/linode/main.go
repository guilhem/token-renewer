package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/guilhem/operator-plugin-framework/client"
	"github.com/guilhem/operator-plugin-framework/stream"
	"github.com/guilhem/token-renewer/shared"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	pluginName    = "linode"
	pluginVersion = "v0.1.0"
)

func main() {
	var (
		operatorAddr    string
		useServiceToken bool
	)

	flag.StringVar(&operatorAddr, "operator-addr", "https://operator-kube-rbac-proxy:8443",
		"Address of the operator gRPC server (via kube-rbac-proxy in production)")
	flag.BoolVar(&useServiceToken, "use-service-token", true,
		"Use Kubernetes ServiceAccount token for authentication (requires kube-rbac-proxy)")

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
		"operator", operatorAddr,
	)

	// Handle graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create Linode plugin instance
	linodePlugin := &LinodePlugin{}

	// Connect to operator using framework client and run plugin
	if err := runPlugin(ctx, operatorAddr, useServiceToken, linodePlugin); err != nil {
		if err != context.Canceled {
			setupLog.Error(err, "plugin failed")
			os.Exit(1)
		}
	}

	setupLog.Info("Plugin stopped gracefully")
}

// runPlugin connects to the operator using the operator-plugin-framework client.
// It establishes a bidirectional stream using the framework's PluginStreamClient, then handles RPC calls.
func runPlugin(ctx context.Context, operatorAddr string, useServiceToken bool, plugin *LinodePlugin) error {
	logger := log.FromContext(ctx)

	// Create client options for authentication
	var clientOpts []client.ClientOption
	if useServiceToken {
		clientOpts = append(clientOpts, client.WithServiceAccountToken())
		logger.Info("Using Kubernetes ServiceAccount token for authentication")
	}

	// Create Plugin Service implementation
	pluginServer := &LinodePlugin{}

	// Create PluginStreamClient using the simplified API
	pluginStreamClient, err := client.New(
		ctx,
		pluginName,
		operatorAddr,
		pluginVersion,
		shared.TokenProviderService_ServiceDesc,
		pluginServer,
		func(conn *grpc.ClientConn) (stream.StreamInterface, error) {
			tokenClient := shared.NewTokenProviderServiceClient(conn)
			return tokenClient.PluginStream(ctx)
		},
		clientOpts...,
	)
	if err != nil {
		return fmt.Errorf("failed to create plugin stream client: %w", err)
	}

	defer func() {
		if cerr := pluginStreamClient.Close(); cerr != nil {
			logger.Error(cerr, "failed to close plugin stream client")
		}
	}()

	logger.Info("Connected to operator and registered plugin via framework")

	// Start handling RPC calls - this blocks until context is cancelled
	return pluginStreamClient.HandleRPCCalls(ctx)
}
