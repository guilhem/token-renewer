package providers

import (
	"net"
	"path/filepath"

	"github.com/hashicorp/go-plugin"

	"github.com/guilhem/token-renewer/shared"
)

func DiscoverPlugins(dir string) (map[string]plugin.ClientConfig, error) {
	// Make the directory absolute if it isn't already
	sockets, err := plugin.Discover("*.socket", dir)
	if err != nil {
		return nil, err
	}

	plugins := make(map[string]plugin.ClientConfig, len(sockets))
	for _, socket := range sockets {
		basename := filepath.Base(socket)
		// Remove the ".socket" suffix to get the plugin name
		pluginName := basename[:len(basename)-len(".socket")]

		addr, err := net.ResolveUnixAddr("unix", socket)
		if err != nil {
			return nil, err
		}
		plugins[socket] = plugin.ClientConfig{
			HandshakeConfig: shared.Handshake,
			Plugins: map[string]plugin.Plugin{
				pluginName: &shared.TokenPlugin{},
			},
			Reattach: &plugin.ReattachConfig{
				Addr: addr,
			},
		}
	}

	return plugins, nil
}
