# Linode Token Renewer Plugin

This plugin provides automatic token renewal for Linode API tokens using the Token Renewer operator.

## Features

- **Automatic Token Renewal**: Creates new Linode API tokens before expiration
- **Non-Expiring Token Handling**: Properly handles tokens without expiration dates (sets 10-year expiration)
- **Bidirectional Streaming**: Uses gRPC streaming for efficient communication with controller
- **Client Architecture**: Connects to token-renewer controller as a client
- **Kubernetes-Native Authentication**: Uses kube-rbac-proxy with ServiceAccount tokens
- **Graceful Shutdown**: Properly handles SIGTERM/SIGINT signals
- **Full Integration**: Uses operator-plugin-framework for standardized plugin architecture

## Architecture

The plugin acts as a **client** that connects to the token-renewer controller via **kube-rbac-proxy**:

```
┌──────────────────────────────────────────────────┐
│  Token Renewer Pod                               │
│  ┌────────────────────────────────────────────┐  │
│  │ kube-rbac-proxy (Sidecar)                  │  │
│  │ - Listens on :8443 (HTTPS)                 │  │
│  │ - Validates ServiceAccount tokens          │  │
│  │ - Proxies to unix socket                   │  │
│  └──────────────┬─────────────────────────────┘  │
│                 │ unix socket                     │
│  ┌──────────────▼─────────────────────────────┐  │
│  │ Controller (gRPC Server)                   │  │
│  │ - Listens on unix:///plugins/*.sock        │  │
│  │ - Sends RenewToken/GetTokenValidity        │  │
│  └────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────┘
                     ▲
                     │ gRPC/HTTPS (authenticated)
                     │
         ┌───────────┴──────────────┐
         │                          │
         ▼                          ▼
┌─────────────────┐         ┌─────────────────┐
│ Linode Plugin   │         │ Other Plugins   │
│ Pod             │         │ Pods            │
│ - Uses SA token │         │ - Uses SA token │
│ - Connects to   │         │ - Connects to   │
│   :8443         │         │   :8443         │
└────────┬────────┘         └─────────────────┘
         │
         ▼
   ┌──────────┐
   │ Linode   │
   │ API      │
   └──────────┘
```

**Flow:**
1. Plugin reads ServiceAccount token from `/var/run/secrets/kubernetes.io/serviceaccount/token`
2. Plugin connects to kube-rbac-proxy at `https://controller-manager:8443`
3. kube-rbac-proxy validates token via Kubernetes TokenReview API
4. kube-rbac-proxy proxies to controller's Unix socket
5. Plugin sends `PluginRegister` message (no auth token needed)
6. Controller sends `RenewTokenRequest` or `GetTokenValidityRequest`
7. Plugin processes request, calls Linode API
8. Plugin sends response back through authenticated stream
9. Controller updates Kubernetes Secret

## Building

```bash
# From plugin directory
go build -o linode .

# Or build Docker image
docker build -t linode-plugin:latest .
```

## Running Locally

```bash
# Build the plugin
go build -o linode .

# Start the plugin client (connects to kube-rbac-proxy)
./linode --server-addr https://controller-manager:8443
```

## Configuration

### Command Line Flags

| Flag            | Default                          | Description                                                              |
| --------------- | -------------------------------- | ------------------------------------------------------------------------ |
| `--server-addr` | `unix:///tmp/token-renewer.sock` | Address of the token-renewer server (use `https://` for kube-rbac-proxy) |

**Note:** The `--auth-secret` flag has been removed. Authentication is handled by kube-rbac-proxy using ServiceAccount tokens.

## Deployment

### Kubernetes Deployment

The plugin is deployed as a **separate pod** that connects through kube-rbac-proxy:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: linode-plugin
  namespace: token-renewer-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: linode-plugin
  namespace: token-renewer-system
spec:
  selector:
    matchLabels:
      app: linode-plugin
  template:
    metadata:
      labels:
        app: linode-plugin
    spec:
      serviceAccountName: linode-plugin
      containers:
      - name: plugin
        image: linode-plugin:latest
        args:
        - --server-addr=https://controller-manager:8443
        env:
        - name: LINODE_TOKEN
          valueFrom:
            secretKeyRef:
              name: linode-credentials
              key: token
```

**RBAC Configuration:**

The plugin needs appropriate RBAC permissions to authenticate with kube-rbac-proxy. See `../../config/rbac/plugin_access_role.yaml` for the complete ClusterRole definition.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: linode-plugin-access
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: plugin-access-role
subjects:
- kind: ServiceAccount
  name: linode-plugin
  namespace: token-renewer-system
```

### Controller Deployment with kube-rbac-proxy

The controller must be deployed with the kube-rbac-proxy sidecar. See `../../config/manager/kube-rbac-proxy.yaml` for complete configuration.

## Usage

### Creating a Token CR

```yaml
apiVersion: token-renewer.barpilot.io/v1beta1
kind: Token
metadata:
  name: my-linode-token
  namespace: default
spec:
  # Reference to Linode provider plugin
  provider: linode
  
  # Linode token ID (as string)
  metadata: "12345"
  
  # Secret where token is stored
  secretRef:
    name: linode-api-token
    namespace: default
  
  # Renewal configuration
  renewal:
    # Renew 24 hours before expiration
    beforeDuration: 24h
```

### Secret Format

The referenced Secret must contain the current Linode token:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: linode-api-token
  namespace: default
type: Opaque
stringData:
  token: "linodeapi-xxxxxxxxxxxxxxxxxxxxx"
```

## Token Lifecycle

1. **Initial State**: Token CR references existing Linode token
2. **Monitoring**: Controller periodically checks token expiration
3. **Renewal Trigger**: When current time > (expiration - beforeDuration):
   - Plugin calls Linode API to get current token details
   - Plugin creates new token with same scopes and labels
   - Plugin deletes old token
   - Controller updates Secret with new token
   - Controller updates Token CR status with new expiration
4. **Requeue**: Controller schedules next reconciliation before new expiration

## Token Metadata

The `metadata` field in the Token CR stores the Linode token ID as a decimal string:

```yaml
spec:
  metadata: "67890"  # Linode token ID
```

After renewal, the controller updates this field with the new token ID.

## Error Handling

### Common Errors

| Error                                               | Cause                               | Solution                                    |
| --------------------------------------------------- | ----------------------------------- | ------------------------------------------- |
| `failed to get token: [401] Invalid Token`          | Current token is invalid or revoked | Manually create new token and update Secret |
| `failed to create token: [429] Rate Limit Exceeded` | Too many API requests               | Wait and retry (controller will auto-retry) |
| `invalid metadata: strconv.Atoi`                    | Metadata is not a valid number      | Check Token CR metadata field format        |

### Non-Expiring Tokens

Linode allows creating tokens without expiration dates. The plugin handles this by:
- Detecting `nil` expiration in API response
- Returning a far-future date (10 years from now)
- Controller treats this as a regular expiration and renews on schedule

This ensures non-expiring tokens still get rotated for security.

## Testing

```bash
# Run all tests
go test -v -race .

# Run specific test
go test -v -run TestPluginServerIntegration

# With coverage
go test -v -race -coverprofile=coverage.out .
go tool cover -html=coverage.out
```

### Test Coverage

- **Integration Tests**: Full gRPC server lifecycle
- **Authentication Tests**: Multiple auth modes (none, secret, serviceaccount)
- **Connection Tests**: Max connections, graceful shutdown
- **Provider Tests**: Metadata conversion, interface compliance
- **API Tests**: Token renewal and validity checking (mocked)

## Development

### Prerequisites

- Go 1.24.2+
- Access to Linode API (for integration testing with real API)
- operator-plugin-framework (included via go.mod replace)

### Project Structure

```
plugins/linode/
├── main.go              # Plugin server entry point
├── linode.go            # Linode TokenProvider implementation
├── linode_test.go       # Unit tests for Linode plugin
├── integration_test.go  # Integration tests with framework
├── go.mod               # Dependencies
├── README.md            # This file
└── deploy/              # Kubernetes deployment configs
    ├── kustomization.yaml
    └── plugin-container-patch.yaml
```

### Adding New Features

1. Implement changes in `linode.go`
2. Add tests in `linode_test.go`
3. Update integration tests if needed
4. Run full test suite: `go test -v -race .`
5. Update this README

## Security Considerations

- **Token Storage**: Tokens are stored in Kubernetes Secrets (base64 encoded)
- **Token Rotation**: Old tokens are deleted immediately after new token creation
- **Plugin Communication**: Uses Unix sockets (file permissions) or TCP with optional auth
- **Linode API**: Uses official `linodego` SDK with HTTPS

### Best Practices

1. **Use minimal scopes**: Only grant required permissions to tokens
2. **Enable renewal**: Set `beforeDuration` to ensure tokens renew before expiration
3. **Monitor status**: Check Token CR status for renewal errors
4. **Audit logs**: Enable controller logging to track token lifecycle

## Troubleshooting

### Plugin Not Connecting

```bash
# Check plugin is running
kubectl get pods -l app=token-renewer

# Check plugin logs
kubectl logs -l app=token-renewer -c linode-plugin

# Verify socket file
kubectl exec -it <pod> -c manager -- ls -la /plugins/
```

### Token Not Renewing

```bash
# Check Token CR status
kubectl describe token my-linode-token

# Check controller logs
kubectl logs -l app=token-renewer -c manager

# Manually trigger reconciliation
kubectl annotate token my-linode-token reconcile=true --overwrite
```

### API Rate Limiting

Linode API has rate limits. If you hit them:
- Increase `beforeDuration` to reduce renewal frequency
- Check for unnecessary reconciliations in controller logs
- Consider using cached token validity checks

## License

Apache License 2.0 - See LICENSE file for details.

## Support

- **Issues**: https://github.com/guilhem/token-renewer/issues
- **Documentation**: https://github.com/guilhem/token-renewer
- **Linode API**: https://www.linode.com/docs/api/
