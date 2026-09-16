// Package searchclient reads Search over gRPC and mandatory mutual TLS.
package searchclient

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"connectrpc.com/connect"
	"github.com/NotaKronGit/travel-watch/api/mtls"
	"github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1/searchv1connect"
)

type Config struct {
	Enabled bool          `mapstructure:"enabled"`
	Address string        `mapstructure:"address"`
	Timeout time.Duration `mapstructure:"timeout"`
	TLS     mtls.Config   `mapstructure:"tls"`
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	u, err := url.Parse(c.Address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || c.Timeout < time.Second || c.Timeout > 5*time.Second {
		return errors.New("invalid Search RPC configuration")
	}
	return c.TLS.Validate()
}
func New(c Config) (searchv1connect.RouteResultsServiceClient, func(), error) {
	if !c.Enabled {
		return nil, func() {}, nil
	}
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	tls, err := c.TLS.Load(false)
	if err != nil {
		return nil, nil, err
	}
	tr := &http.Transport{TLSClientConfig: tls, ForceAttemptHTTP2: true, DialContext: (&net.Dialer{Timeout: c.Timeout}).DialContext, TLSHandshakeTimeout: c.Timeout, ResponseHeaderTimeout: c.Timeout, IdleConnTimeout: time.Minute, MaxIdleConns: 10, MaxConnsPerHost: 10}
	client := &http.Client{Transport: tr, Timeout: c.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return searchv1connect.NewRouteResultsServiceClient(client, c.Address, connect.WithGRPC(), connect.WithReadMaxBytes(2<<20)), tr.CloseIdleConnections, nil
}
