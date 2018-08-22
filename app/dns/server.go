package dns

//go:generate go run $GOPATH/src/v2ray.com/core/common/errors/errorgen/main.go -pkg dns -path App,DNS

import (
	"context"
	"sync"
	"time"

	"v2ray.com/core"
	"v2ray.com/core/common"
	"v2ray.com/core/common/net"
)

type Server struct {
	sync.Mutex
	hosts    *StaticHosts
	servers  []NameServer
	clientIP net.IP
}

func New(ctx context.Context, config *Config) (*Server, error) {
	server := &Server{
		servers: make([]NameServer, len(config.NameServers)),
	}
	if len(config.ClientIp) > 0 {
		if len(config.ClientIp) != 4 && len(config.ClientIp) != 16 {
			return nil, newError("unexpected IP length", len(config.ClientIp))
		}
		server.clientIP = net.IP(config.ClientIp)
	}

	hosts, err := NewStaticHosts(config.StaticHosts, config.Hosts)
	if err != nil {
		return nil, newError("failed to create hosts").Base(err)
	}
	server.hosts = hosts

	v := core.MustFromContext(ctx)
	if err := v.RegisterFeature((*core.DNSClient)(nil), server); err != nil {
		return nil, newError("unable to register DNSClient.").Base(err)
	}

	for idx, destPB := range config.NameServers {
		address := destPB.Address.AsAddress()
		if address.Family().IsDomain() && address.Domain() == "localhost" {
			server.servers[idx] = NewLocalNameServer()
		} else {
			dest := destPB.AsDestination()
			if dest.Network == net.Network_Unknown {
				dest.Network = net.Network_UDP
			}
			if dest.Network == net.Network_UDP {
				server.servers[idx] = NewClassicNameServer(dest, v.Dispatcher(), server.clientIP)
			}
		}
	}
	if len(config.NameServers) == 0 {
		server.servers = append(server.servers, NewLocalNameServer())
	}

	return server, nil
}

// Start implements common.Runnable.
func (s *Server) Start() error {
	return nil
}

// Close implements common.Closable.
func (s *Server) Close() error {
	return nil
}

func (s *Server) LookupIP(domain string) ([]net.IP, error) {
	if ip := s.hosts.LookupIP(domain); len(ip) > 0 {
		return ip, nil
	}

	var lastErr error
	for _, server := range s.servers {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*4)
		ips, err := server.QueryIP(ctx, domain)
		cancel()
		if err != nil {
			lastErr = err
		}
		if len(ips) > 0 {
			return ips, nil
		}
	}

	return nil, newError("returning nil for domain ", domain).Base(lastErr)
}

func init() {
	common.Must(common.RegisterConfig((*Config)(nil), func(ctx context.Context, config interface{}) (interface{}, error) {
		return New(ctx, config.(*Config))
	}))
}
