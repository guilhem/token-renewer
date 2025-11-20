# Token Renewer

A Kubernetes operator that automatically renews API tokens for various cloud providers through a pluggable architecture.

## Description

Token Renewer is a Kubernetes controller built with Kubebuilder that manages the lifecycle of API tokens stored in Kubernetes Secrets. It automatically renews tokens before they expire using provider-specific plugins that communicate via gRPC over Unix domain sockets.

**Key Features:**
- **Automatic Token Renewal**: Monitors token expiration and renews them before they become invalid
- **Plugin Architecture**: Extensible design allowing custom providers through gRPC plugins
- **Kubernetes-Native**: Manages tokens using Custom Resources and Secrets
- **Multi-Provider Support**: Each provider runs as an independent plugin (e.g., Linode, AWS, GCP)
- **Event Logging**: Emits Kubernetes events for visibility into token operations

**How It Works:**
1. Create a `Token` custom resource that references a Kubernetes Secret and specifies a provider
2. The controller discovers available provider plugins from Unix sockets in `/plugins/*.sock`
3. On reconciliation, the controller checks token validity and renews it when approaching expiration
4. The renewed token is stored back in the referenced Secret
5. The controller requeues based on the next expiration time minus the configured renewal buffer

## Getting Started

### Prerequisites
- Go version v1.23.0+
- Docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster
- (For development) buf CLI for Protocol Buffer generation

### Quick Start with Linode Example

The fastest way to get started is using the Linode provider example:

1. **Create a Linode API token Secret:**
```sh
kubectl create secret generic linode-token --from-literal=token=YOUR_LINODE_TOKEN
```

2. **Deploy the operator with Linode plugin:**
```sh
kubectl apply -k examples/linode/
```

3. **Create a Token resource:**
```sh
kubectl apply -f examples/linode/token.yaml
```

4. **Monitor the token renewal:**
```sh
kubectl get tokens -w
kubectl describe token <token-name>
```

### Deploy on the Cluster

**1. Build and push your image:**

```sh
make docker-build docker-push IMG=<your-registry>/token-renewer:tag
```

> **Note:** Ensure you have push access to the registry and the cluster can pull from it.

**2. Install the CRDs:**

```sh
make install
```

**3. Deploy the controller:**

```sh
make deploy IMG=<your-registry>/token-renewer:tag
```

> **Note:** If you encounter RBAC errors, you may need cluster-admin privileges.

**4. Create a Token resource:**

Create a Secret with your initial token:
```sh
kubectl create secret generic my-token --from-literal=token=YOUR_API_TOKEN
```

Apply a Token CR (replace values as needed):
```yaml
apiVersion: token-renewer.barpilot.io/v1beta1
kind: Token
metadata:
  name: my-token
spec:
  provider:
    name: linode  # or your custom provider
  metadata: "12345"  # Provider-specific ID (e.g., Linode token ID)
  renewval:
    beforeDuration: 24h  # Renew 24 hours before expiration
  secretRef:
    name: my-token
```

```sh
kubectl apply -f token.yaml
```

### Uninstall

**1. Delete Token resources:**

```sh
kubectl delete tokens --all
```

**2. Undeploy the controller:**

```sh
make undeploy
```

**3. Delete the CRDs:**

```sh
make uninstall
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/token-renewer:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/token-renewer/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Developing Plugins

Token Renewer uses a plugin architecture where each provider runs as a separate process communicating via gRPC over Unix domain sockets.

### Creating a New Provider Plugin

1. **Create plugin directory:**
```sh
mkdir -p plugins/myprovider
cd plugins/myprovider
```

2. **Implement the `TokenProvider` interface:**

```go
package main

import (
    "context"
    "time"
    "github.com/guilhem/token-renewer/shared"
)

type MyProviderPlugin struct{}

func (p *MyProviderPlugin) RenewToken(ctx context.Context, metadata, token string) (string, string, *time.Time, error) {
    // 1. Use the current token to authenticate with your provider's API
    // 2. Create a new token
    // 3. Delete the old token (if necessary)
    // 4. Return: newToken, newMetadata, expirationTime, error
    return newToken, newMetadata, &expirationTime, nil
}

func (p *MyProviderPlugin) GetTokenValidity(ctx context.Context, metadata, token string) (*time.Time, error) {
    // Query your provider's API to get the token's expiration time
    return &expirationTime, nil
}
```

3. **Create the plugin main function:**

```go
func main() {
    plugin := &MyProviderPlugin{}
    server := shared.GRPCServer{Impl: plugin}
    
    dir := os.Getenv("PLUGIN_DIR")
    if dir == "" {
        dir = "/plugins"
    }
    
    socket := filepath.Join(dir, "myprovider.sock")
    defer os.Remove(socket)
    
    lis, err := net.Listen("unix", socket)
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }
    defer lis.Close()
    
    grpcServer := grpc.NewServer()
    shared.RegisterTokenProviderServiceServer(grpcServer, &server)
    grpcServer.Serve(lis)
}
```

4. **Deploy alongside the controller:**

See `examples/linode/` for a complete example using Kustomize patches to add a sidecar container.

### Development Workflow

**Run tests:**
```sh
make test              # Unit tests
make test-e2e          # E2E tests (requires Kind cluster)
```

**Run locally:**
```sh
make run
```

**Lint and format:**
```sh
make lint              # Run golangci-lint
make fmt vet           # Format and vet code
```

**Generate code after API changes:**
```sh
make manifests generate
```

**Update Protocol Buffers:**
```sh
buf generate
```

## Architecture

### Components

- **Controller**: Watches `Token` CRs, manages reconciliation and token renewal
- **Providers Manager**: Discovers and registers gRPC plugin clients
- **Plugins**: Provider-specific implementations running as separate processes
- **gRPC Interface**: Protocol Buffer-defined service for `RenewToken` and `GetTokenValidity`

### Token Custom Resource

```yaml
apiVersion: token-renewer.barpilot.io/v1beta1
kind: Token
metadata:
  name: example-token
spec:
  provider:
    name: linode              # Plugin name (matches socket filename)
  metadata: "12345"           # Provider-specific identifier
  renewval:
    beforeDuration: 24h       # Renew this long before expiration
  secretRef:
    name: my-secret           # Secret containing the token
status:
  expirationTime: "2025-12-01T00:00:00Z"  # Managed by controller
```

### Configuration

The controller accepts the following flags:

- `--sockets-plugins-dir`: Directory containing plugin sockets (default: `/plugins`)
- `--leader-elect`: Enable leader election for HA deployments
- `--metrics-bind-address`: Metrics endpoint address (default: `0`)
- `--health-probe-bind-address`: Health probe endpoint (default: `:8081`)

## Contributing

Contributions are welcome! Here's how to get started:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes and add tests
4. Run tests and linting (`make test lint`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to your branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

**Development Guidelines:**
- Follow idiomatic Go practices
- Add tests for new functionality
- Update documentation for API changes
- Run `make manifests generate` after modifying CRDs
- Ensure all tests pass before submitting PR

**NOTE:** Run `make help` for more information on all available `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
