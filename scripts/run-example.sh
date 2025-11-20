#!/bin/bash
# Example: Running token-renewer with plugin-initiated architecture

set -e

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}/sources/token-renewer"

# Ensure we're in the right directory
cd "$REPO_ROOT"

# Function to print colored output
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to cleanup on exit
cleanup() {
    log_info "Cleaning up..."
    
    # Kill background processes
    jobs -p | xargs -r kill 2>/dev/null || true
    
    # Remove socket files
    rm -f /tmp/token-renewer.sock /tmp/linode-plugin.sock 2>/dev/null || true
}

trap cleanup EXIT

# Parse arguments
MODE="${1:-dev-no-auth}"

case "$MODE" in
    dev-no-auth)
        log_info "Starting in development mode (no authentication)"
        
        # Build
        log_info "Building controller..."
        go build -o /tmp/controller ./cmd/main.go
        
        log_info "Building Linode plugin..."
        go build -o /tmp/linode-plugin ./plugins/linode/main.go
        
        # Start controller
        log_info "Starting controller on unix:///tmp/token-renewer.sock"
        /tmp/controller \
            --plugin-server-addr=unix:///tmp/token-renewer.sock \
            --plugin-auth-mode=none \
            --health-probe-bind-address=:8081 \
            --metrics-bind-address=0 &
        CONTROLLER_PID=$!
        
        # Give controller time to start
        sleep 1
        
        # Start plugin
        log_info "Starting Linode plugin..."
        /tmp/linode-plugin \
            --server-addr=unix:///tmp/token-renewer.sock \
            --plugin-name=linode \
            --socket-addr=unix:///tmp/linode-plugin.sock &
        PLUGIN_PID=$!
        
        log_info "Controller PID: $CONTROLLER_PID"
        log_info "Plugin PID: $PLUGIN_PID"
        log_info "Press Ctrl+C to stop"
        
        # Wait for processes
        wait $CONTROLLER_PID $PLUGIN_PID
        ;;
        
    dev-secret)
        log_info "Starting in development mode (with shared secret)"
        
        SHARED_SECRET="${2:-my-secret-123}"
        
        # Build
        log_info "Building controller and plugin..."
        go build -o /tmp/controller ./cmd/main.go
        go build -o /tmp/linode-plugin ./plugins/linode/main.go
        
        # Start controller with shared secret
        log_info "Starting controller with shared secret"
        PLUGIN_AUTH_SECRET="$SHARED_SECRET" \
        /tmp/controller \
            --plugin-server-addr=tcp://127.0.0.1:50051 \
            --plugin-auth-mode=secret \
            --health-probe-bind-address=:8081 \
            --metrics-bind-address=0 &
        CONTROLLER_PID=$!
        
        sleep 1
        
        # Start plugin
        log_info "Starting Linode plugin with shared secret"
        /tmp/linode-plugin \
            --server-addr=tcp://127.0.0.1:50051 \
            --plugin-name=linode \
            --auth-token="$SHARED_SECRET" \
            --tcp-addr=127.0.0.1:50052 &
        PLUGIN_PID=$!
        
        log_info "Controller PID: $CONTROLLER_PID (TCP 50051)"
        log_info "Plugin PID: $PLUGIN_PID (TCP 50052)"
        log_info "Shared Secret: $SHARED_SECRET"
        log_info "Press Ctrl+C to stop"
        
        wait $CONTROLLER_PID $PLUGIN_PID
        ;;
        
    test)
        log_info "Running tests..."
        go test ./internal/pluginserver -v
        go test ./internal/controller -v
        ;;
        
    build)
        log_info "Building binaries..."
        go build -o /tmp/controller ./cmd/main.go
        go build -o /tmp/linode-plugin ./plugins/linode/main.go
        log_info "Built to /tmp/controller and /tmp/linode-plugin"
        ;;
        
    *)
        log_error "Unknown mode: $MODE"
        echo ""
        echo "Usage: $0 <mode> [args]"
        echo ""
        echo "Modes:"
        echo "  dev-no-auth           Start with no authentication (default)"
        echo "  dev-secret [secret]   Start with shared secret (default: my-secret-123)"
        echo "  test                  Run unit tests"
        echo "  build                 Build binaries only"
        echo ""
        echo "Examples:"
        echo "  $0 dev-no-auth"
        echo "  $0 dev-secret secure-token-xyz"
        echo "  $0 test"
        exit 1
        ;;
esac
