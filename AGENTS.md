# Token Renewer - AI Agent Instructions

## Project Overview

Token Renewer is a **Kubernetes operator** (built with Kubebuilder v4) that automatically renews API tokens through a **gRPC plugin architecture**. Plugins run as Unix socket servers; the controller discovers them dynamically from `/plugins/*.sock` and communicates via Protocol Buffers.

**Core Flow:**
1. User creates a `Token` CR referencing a Secret and provider plugin
2. Controller watches Token CRs, discovers plugins from socket directory
3. On reconciliation, controller calls plugin's gRPC methods to validate/renew tokens
4. Controller updates Secret with new token and requeues before expiration

**Key Components:**
- `api/v1beta1/token_types.go`: Token CRD with `provider`, `metadata`, `renewval`, and `secretRef`
- `internal/controller/token_controller.go`: Reconciler that manages token lifecycle
- `internal/providers/`: Plugin discovery (`*.sock` glob) and manager registry (uses shared `operator-plugin-framework`)
- `shared/`: gRPC client/server wrappers implementing `TokenProvider` interface
- `proto/`: Protobuf definitions for `RenewToken` and `GetTokenValidity` RPCs
- `plugins/*/`: Individual provider implementations (e.g., `linode`)
- **NEW:** `operator-plugin-framework/`: Reusable plugin framework for Kubernetes operators (available for extraction as separate module)

## Architecture Patterns

### Plugin Communication
- **Protocol:** gRPC over Unix domain sockets (`unix:///plugins/<name>.sock`)
- **Discovery:** Startup scans `/plugins` directory (configurable via `--sockets-plugins-dir`)
- **Interface:** All plugins implement `shared.TokenProvider` (2 methods: `RenewToken`, `GetTokenValidity`)
- **Lifecycle:** Plugins are separate binaries/containers deployed alongside the controller

### Controller Logic
- **Reconciliation Strategy:** Time-based requeue using `token.Status.ExpirationTime - token.Spec.Renewval.BeforeDuration`
- **State Management:** `ExpirationTime` in status; controller sets it on first reconcile if empty
- **Secret Updates:** Uses `controllerutil.CreateOrUpdate` to modify referenced Secret's `data.token` field
- **Token Rotation:** On renewal, updates both Secret (new token) and Token CR (new metadata + expiration)

### Kubebuilder Scaffolding
- **Generated Code:** Run `make generate` (DeepCopy methods) and `make manifests` (CRDs/RBAC) after API changes
- **Tool Versions:** Pinned in Makefile (e.g., `CONTROLLER_TOOLS_VERSION ?= v0.17.2`)
- **RBAC Markers:** `+kubebuilder:rbac` comments in controller files generate role permissions

## Development Workflow

### ⚠️ CRITICAL: Always Use `make` for All Tasks

**MANDATORY RULE:** Never use `go test`, `go run`, `go build`, or direct `kubectl` commands. Always use `make` targets defined in `Makefile`. This ensures:
- Consistent environment setup (code generation, formatting, linting)
- Proper build isolation and reproducibility
- Correct handling of dependencies and tool versions
- Compliance with project standards

### Essential Commands
```bash
# Code generation (required after changing api/v1beta1/*)
make manifests generate

# Build & run locally (connects to current kubeconfig context)
make run

# Testing - USE THESE, NEVER go test DIRECTLY
make test                              # Unit tests (uses envtest for fake K8s API)
make test-e2e                          # E2E tests (requires running Kind cluster)
make test-e2e CERT_MANAGER_INSTALL_SKIP=true  # E2E tests without cert-manager setup

# Linting
make lint                              # Run golangci-lint
make fmt vet                           # Format and vet code

# Deployment
make docker-build IMG=<registry>/token-renewer:tag
make deploy IMG=<registry>/token-renewer:tag
make install                           # Install CRDs
make uninstall                         # Uninstall CRDs
make undeploy                          # Undeploy controller

# Kind cluster management
make kind-reset                        # Delete and recreate Kind cluster (reset state)
```

### Plugin Development
1. Create directory in `plugins/<provider-name>/`
2. Implement `shared.TokenProvider` interface (see `plugins/linode/linode.go`)
3. Main function: create Unix socket at `filepath.Join(PLUGIN_DIR, "<name>.sock")`
4. Register with `shared.RegisterTokenProviderServiceServer`
5. Build plugin binary and deploy as sidecar container

**Example Pattern (from Linode):**
- Metadata stores provider-specific ID (e.g., Linode token ID as string)
- `RenewToken`: Create new token, delete old, return new token + ID + expiration
- `GetTokenValidity`: Query provider API for expiration time

### Protocol Buffer Changes
```bash
# Generate Go code from proto files (buf is used)
buf generate
# Output: shared/*.pb.go files
```

## Project-Specific Conventions

### Naming & Structure
- **CRD Group:** `token-renewer.barpilot.io` (domain in PROJECT file)
- **API Version:** `v1beta1` (single version; consider v1 after stabilization)
- **Provider Registry:** `ProvidersManager` in `internal/providers/manager.go` maps plugin names to gRPC clients

### Error Handling
- Controller uses `client.IgnoreNotFound(err)` for missing CRs (deleted during reconciliation)
- Plugin errors propagated as standard Go errors; controller logs and records Events
- Always emit Events (`r.Recorder.Event`) for user-facing errors (e.g., "SecretNotFound")

### Testing Strategy
- **Unit Tests:** Ginkgo/Gomega style in `*_test.go` files (see `token_controller_test.go`)
- **Setup:** `suite_test.go` creates envtest client; tests use `k8sClient` from package scope
- **E2E Tests:** Require Kind cluster; test full deployment including plugin communication

## Integration Points

### Kubernetes Resources
- **Watches:** Token CR only (no watches on Secrets; reconciliation triggered by CR changes/timers)
- **Secrets:** Controller has full RBAC (`get;list;watch;create;update;patch;delete`) to manage token storage
- **Events:** Emitted for renewal success/failure; visible via `kubectl describe token`

### External Dependencies
- **Plugin Sockets:** Must be accessible at runtime (typically via shared volume in Pod)
- **Provider APIs:** Plugins make external HTTP calls (e.g., Linode uses `linodego` SDK)

### Configuration
- **Flags:** See `cmd/main.go` for all CLI options (metrics, webhooks, leader election)
- **Key Flags:**
  - `--sockets-plugins-dir=/plugins`: Plugin discovery directory
  - `--leader-elect=false`: Enable for multi-replica deployments
  - `--metrics-bind-address=:8443`: Secure metrics endpoint

## Integrated Framework: operator-plugin-framework

Token Renewer includes a **reusable plugin framework** (`operator-plugin-framework/`) that can be extracted and used by other Kubernetes operators. This framework handles the generic infrastructure for bidirectional gRPC plugin communication.

### Framework Features
- **Thread-safe plugin registry** (`registry/manager.go`): Register, unregister, and retrieve plugins
- **Server lifecycle management** (`server/server.go`): gRPC server initialization and shutdown
- **Configurable options** (`server/options.go`): Authentication modes (none, secret, serviceaccount) and connection limits
- **Client helpers** (`client/client.go`): Simplified plugin connection management

### Framework Usage in Token Renewer
- `internal/providers/manager.go`: Wraps `registry.Manager` to provide TokenProvider-specific interface
- `internal/pluginserver/server.go`: Extends framework server with bidirectional streaming logic for RPC communication
- `cmd/main.go`: Discovers and manages plugins using the shared framework

### Extracting Framework to Separate Module
The framework is currently embedded in the token-renewer repository but can be extracted as a standalone Go module:
1. Move `operator-plugin-framework/` to a separate repository
2. Update `go.mod` in token-renewer to reference external module: `replace` → `require` with GitHub URL
3. Other operators can then import and use the framework for their own plugin systems

### Integration Pattern (for Other Operators)
```go
import "github.com/your-org/operator-plugin-framework/registry"

// Create plugin registry
manager := registry.New()

// Register a plugin (must implement PluginProvider interface)
manager.Register("my-plugin", myPluginImpl)

// Retrieve plugin
plugin, err := manager.Get("my-plugin")
```

## Common Pitfalls

1. **Forgetting `make manifests generate`** after API changes → stale CRDs/generated code
2. **Plugin socket naming:** Must match glob `*.sock` (note: `discover.go` has bug - checks `.socket` suffix but should be `.sock`)
3. **Metadata format:** Provider-specific; document format in plugin README (Linode uses token ID as decimal string)
4. **Requeue timing:** Controller calculates `RequeueAfter` from expiration minus renewal duration; ensure `BeforeDuration` gives adequate buffer
5. **gRPC client lifecycle:** Connections created once at startup; no reconnection logic if plugins restart

## Key Files Reference

- **Entry Point:** `cmd/main.go` (plugin discovery happens here)
- **Core Types:** `api/v1beta1/token_types.go`
- **Reconciliation:** `internal/controller/token_controller.go`
- **Plugin Interface:** `shared/token_provider.go`
- **Example Plugin:** `plugins/linode/` (complete reference implementation)
- **Deployment Config:** `config/` (Kustomize manifests for CRD/RBAC/deployment)
- **Build Config:** `Makefile`, `Dockerfile`, `buf.gen.yaml` (protobuf generation)
- **Framework:** `operator-plugin-framework/` (reusable plugin infrastructure - can be extracted as separate module)
