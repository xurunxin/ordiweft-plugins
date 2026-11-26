package tapnow

import (
	"context"
	"gitea.nkit.top/xurx/Ordiweft/internal/mediafetch"
	"net"
)

func lookupMediaIPs(ctx context.Context, host string) ([]net.IPAddr, error) {
	return mediafetch.LookupIPs(ctx, host)
}
