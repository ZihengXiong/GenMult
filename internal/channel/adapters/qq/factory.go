package qq

import (
	"log/slog"

	"github.com/ZihengXiong/GenMult/internal/channel"
	"github.com/ZihengXiong/GenMult/internal/channel/identities"
	"github.com/ZihengXiong/GenMult/internal/channel/route"
	"github.com/ZihengXiong/GenMult/internal/media"
)

func ProvideQQAdapter(log *slog.Logger, mediaService *media.Service, identityService *identities.Service, routeService *route.DBService) channel.Adapter {
	adapter := NewQQAdapter(log)
	adapter.SetAssetOpener(mediaService)
	adapter.SetChannelIdentityResolver(identityService)
	adapter.SetRouteResolver(routeService)
	return adapter
}
