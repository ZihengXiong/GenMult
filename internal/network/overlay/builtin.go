package overlay

import (
	netctl "github.com/ZihengXiong/GenMult/internal/network"
	"github.com/ZihengXiong/GenMult/internal/network/overlay/netbird"
	"github.com/ZihengXiong/GenMult/internal/network/overlay/tailscale"
)

func RegisterBuiltinProviders(registry *netctl.Registry, deps ProviderDeps) error {
	if registry == nil {
		return nil
	}
	providers := []netctl.Provider{
		tailscale.NewProvider(tailscale.Deps{
			SidecarRuntime: deps.SidecarRuntime,
			KubeRuntime:    deps.KubeRuntime,
			Runtime:        deps.Runtime,
			StateRoot:      deps.StateRoot,
		}),
		netbird.NewProvider(netbird.Deps{
			SidecarRuntime: deps.SidecarRuntime,
			KubeRuntime:    deps.KubeRuntime,
			Runtime:        deps.Runtime,
			StateRoot:      deps.StateRoot,
		}),
	}
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			return err
		}
	}
	return nil
}
