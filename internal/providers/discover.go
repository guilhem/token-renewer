package providers

import (
	"fmt"
	"path/filepath"

	"google.golang.org/grpc"

	"github.com/guilhem/token-renewer/shared"
)

func DiscoverPlugins(dir string) (map[string]*shared.GRPCClient, error) {

	if !filepath.IsAbs(dir) {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		dir = absDir
	}

	sockets, err := filepath.Glob(filepath.Join(dir, "*.sock"))
	if err != nil {
		return nil, err
	}

	plugins := make(map[string]*shared.GRPCClient, len(sockets))
	for _, socket := range sockets {
		socketName := filepath.Base(socket)
		// Remove the ".socket" suffix to get the plugin name
		pluginName := socketName[:len(socketName)-len(".socket")]

		conn, err := grpc.NewClient(fmt.Sprintf("unix://%s", socket))
		if err != nil {
			return nil, err
		}
		plugins[pluginName] = shared.NewGRPCClient(shared.NewTokenProviderServiceClient(conn))
	}

	return plugins, nil
}
