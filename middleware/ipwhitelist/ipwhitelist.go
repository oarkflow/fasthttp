package ipwhitelist

import (
	"net"
	"strings"

	fh "github.com/orgware/fasthttp"
)

func New(allowed ...string) fh.HandlerFunc {
	networks := make([]*net.IPNet, 0, len(allowed))
	ips := make([]net.IP, 0, len(allowed))

	for _, a := range allowed {
		if strings.Contains(a, "/") {
			_, n, err := net.ParseCIDR(a)
			if err == nil {
				networks = append(networks, n)
			}
		} else {
			if ip := net.ParseIP(a); ip != nil {
				ips = append(ips, ip)
			}
		}
	}

	return func(ctx *fh.Ctx) error {
		clientIP := net.ParseIP(ctx.IP())
		if clientIP == nil {
			return ctx.Status(403).SendString("Forbidden")
		}
		for _, ip := range ips {
			if ip.Equal(clientIP) {
				return ctx.Next()
			}
		}
		for _, n := range networks {
			if n.Contains(clientIP) {
				return ctx.Next()
			}
		}
		return ctx.Status(403).SendString("Forbidden")
	}
}
