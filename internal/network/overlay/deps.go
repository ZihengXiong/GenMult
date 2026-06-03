package overlay

import (
	netctl "github.com/ZihengXiong/GenMult/internal/network"
	"github.com/ZihengXiong/GenMult/internal/network/kubeapi"
	"github.com/ZihengXiong/GenMult/internal/network/overlay/internal/sidecar"
)

type ProviderDeps struct {
	SidecarRuntime sidecar.Runtime
	KubeRuntime    kubeapi.Runtime
	Runtime        netctl.RuntimeDescriptor
	StateRoot      string
}
