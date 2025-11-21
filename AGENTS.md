# Token Renewer - AI Agent Instructions

## Project Overview

Token Renewer is a **Kubernetes operator** (built with Kubebuilder v4) that automatically renews API tokens through a **gRPC plugin architecture**. Plugins connect dynamically via the **operator-plugin-framework**, communicating securely through kube-rbac-proxy via Protocol Buffers.

**Core Flow:**
1. User creates a `Token` CR referencing a Secret and provider plugin
2. Operator starts gRPC server listening for plugin connections
3. Plugins connect via HTTPS (gRPC/TLS) through kube-rbac-proxy with ServiceAccount token authentication
4. On reconciliation, controller queries plugin registry and calls plugin's gRPC methods to validate/renew tokens
5. Controller updates Secret with new token and requeues before expiration

**Key Components:**
- `api/v1beta1/token_types.go`: Token CRD with `provider`, `metadata`, `renewval`, and `secretRef`
- `internal/controller/token_controller.go`: Reconciler that manages token lifecycle
- `internal/providers/`: Provider manager wrapping operator-plugin-framework
- `internal/pluginserver/`: gRPC server setup extending operator-plugin-framework
- `shared/`: gRPC service definitions (generated from protobuf)
- `proto/`: Protobuf definitions for `RenewToken` and `GetTokenValidity` RPCs
- `plugins/*/`: Individual provider implementations (e.g., `linode`)
- `config/`: Kustomize manifests for deployment, RBAC, and kube-rbac-proxy configuration

## Architecture Patterns

### Plugin Communication
- **Protocol:** gRPC over HTTPS (TLS) via kube-rbac-proxy sidecar, forwarded to Unix socket
- **Connection Flow:** Plugin → HTTPS (gRPC/TLS) → kube-rbac-proxy → Unix socket → Operator
- **Authentication:** ServiceAccount tokens validated by kube-rbac-proxy against RBAC policies
- **Registration:** Plugins register on connection via bidirectional gRPC stream (automatic via framework)
- **Interface:** All plugins implement gRPC service defined in `proto/`
- **Lifecycle:** Plugins are separate containers deployed with kube-rbac-proxy sidecar for secure communication
- **Framework:** Uses `operator-plugin-framework` for server, registry, and client management

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
2. Implement gRPC service defined in `proto/` (e.g., `TokenProviderService`)
3. Main function: create gRPC server listening on Unix socket
4. Connect to operator's gRPC server (via kube-rbac-proxy) to register and stream
5. Build plugin binary and deploy as separate container with kube-rbac-proxy sidecar
6. Configure RBAC: ClusterRole with `nonResourceURLs: ["/"]` to allow plugin access

**Example Pattern (from Linode):**
- Metadata stores provider-specific ID (e.g., Linode token ID as string)
- `RenewToken`: Create new token, delete old, return new token + ID + expiration
- `GetTokenValidity`: Query provider API for expiration time
- Plugin connects via: `https://operator-kube-rbac-proxy:8443` with ServiceAccount token auth

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
  - `--metrics-bind-address=:8443`: Secure metrics endpoint
  - `--leader-elect=false`: Enable for multi-replica deployments
  - Framework handles plugin server initialization automatically

## Integrated Framework: operator-plugin-framework

Token Renewer uses the **operator-plugin-framework** (available as a separate repository at `github.com/guilhem/operator-plugin-framework`) for its plugin communication infrastructure. This framework provides reusable components for bidirectional gRPC plugin communication with kube-rbac-proxy authentication.

### Framework Integration
- **Server:** Uses framework's gRPC server for managing plugin connections
- **Registry:** Thread-safe plugin registry for registering and retrieving plugins
- **Client:** Framework's client helpers for plugin connection management
- **Authentication:** Leverages kube-rbac-proxy sidecar for secure token validation

### Token Renewer Customizations
- `internal/providers/manager.go`: Wraps framework components to provide TokenProvider-specific interface
- `internal/pluginserver/server.go`: Extends framework server with token renewal logic
- `cmd/main.go`: Plugin discovery and lifecycle management
- `config/manager/kube-rbac-proxy.yaml`: Deploys kube-rbac-proxy sidecar
- `config/rbac/kube-rbac-proxy_role.yaml`: RBAC for token validation
- `config/rbac/plugin_access_role.yaml`: RBAC for plugin access control

## Common Pitfalls

1. **Forgetting `make manifests generate`** after API changes → stale CRDs/generated code
2. **RBAC Configuration:** Must configure both operator-side (TokenReview/SubjectAccessReview) and plugin-side (nonResourceURLs RBAC) permissions
3. **kube-rbac-proxy flags:** Ensure `--secure-listen-address` and `--upstream` are correctly configured
4. **Metadata format:** Provider-specific; document format in plugin README (Linode uses token ID as decimal string)
5. **Requeue timing:** Controller calculates `RequeueAfter` from expiration minus renewal duration; ensure `BeforeDuration` gives adequate buffer
6. **gRPC client lifecycle:** Plugins must maintain persistent connections to kube-rbac-proxy; implement reconnection logic
7. **ServiceAccount tokens:** Ensure plugin ServiceAccount exists and has correct RBAC bindings for accessing operator endpoint

## Key Files Reference

- **Entry Point:** `cmd/main.go` (plugin discovery happens here)
- **Core Types:** `api/v1beta1/token_types.go`
- **Reconciliation:** `internal/controller/token_controller.go`
- **Plugin Interface:** `shared/token_provider.go`
- **Example Plugin:** `plugins/linode/` (complete reference implementation)
- **Deployment Config:** `config/` (Kustomize manifests for CRD/RBAC/deployment)
- **Build Config:** `Makefile`, `Dockerfile`, `buf.gen.yaml` (protobuf generation)
- **Framework:** Uses `github.com/guilhem/operator-plugin-framework` for plugin communication (separate repository)
