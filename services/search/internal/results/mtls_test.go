package results

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/NotaKronGit/travel-watch/api/mtls"
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1/searchv1connect"
	"github.com/NotaKronGit/travel-watch/internal/devpki"
)

type testStore struct{ calls atomic.Int32 }

func (s *testStore) ReadRoutes(context.Context, *v1.GetRoutesRequest) (*v1.GetRoutesResponse, error) {
	s.calls.Add(1)
	return &v1.GetRoutesResponse{Revision: 7, Sources: []*v1.SourceRoutes{{PlannerId: "graph", Total: 1, Routes: []*v1.RouteScheme{{Steps: []*v1.RouteStep{{Description: "Synthetic A → B"}}}}}}}, nil
}
func TestGRPCMutualTLSAndIdentity(t *testing.T) {
	ca, err := devpki.New()
	if err != nil {
		t.Fatal(err)
	}
	serverCfg, err := ca.Issue("search", "cabinet", true)
	if err != nil {
		t.Fatal(err)
	}
	clientCfg, err := ca.Issue("cabinet", "search", false)
	if err != nil {
		t.Fatal(err)
	}
	tls, err := serverCfg.Load(true)
	if err != nil {
		t.Fatal(err)
	}
	store := &testStore{}
	srv := httptest.NewUnstartedServer(Handler(store, time.Second))
	srv.EnableHTTP2 = true
	srv.TLS = tls
	srv.StartTLS()
	defer srv.Close()
	request := &v1.GetRoutesRequest{RequestId: "11111111-1111-4111-8111-111111111111", PageSize: 5}
	call := func(c mtls.Config, noCert bool) (*connect.Response[v1.GetRoutesResponse], error) {
		tls, err := c.Load(false)
		if err != nil {
			return nil, err
		}
		if noCert {
			tls.Certificates = nil
		}
		tr := &http.Transport{TLSClientConfig: tls, ForceAttemptHTTP2: true}
		defer tr.CloseIdleConnections()
		client := searchv1connect.NewRouteResultsServiceClient(&http.Client{Transport: tr, Timeout: time.Second}, srv.URL, connect.WithGRPC())
		return client.GetRoutes(context.Background(), connect.NewRequest(request))
	}
	response, err := call(clientCfg, false)
	if err != nil || response.Msg.Revision != 7 {
		t.Fatal("valid gRPC/mTLS", err)
	}
	if _, err = call(clientCfg, true); err == nil {
		t.Fatal("client without certificate admitted")
	}
	other, err := ca.Issue("collector", "search", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = call(other, false); err == nil {
		t.Fatal("different identity from same CA admitted")
	}
	badName := clientCfg
	badName.PeerName = "wrong-server"
	if _, err = call(badName, false); err == nil {
		t.Fatal("server identity not verified")
	}
	otherCA, err := devpki.New()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := otherCA.Issue("cabinet", "search", false)
	if err != nil {
		t.Fatal(err)
	}
	foreign.CABase64 = clientCfg.CABase64
	if _, err = call(foreign, false); err == nil {
		t.Fatal("untrusted client CA admitted")
	}
	if store.calls.Load() != 1 {
		t.Fatal("rejected peers reached repository")
	}
	request.Offset = -1
	if _, err = call(clientCfg, false); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatal("invalid pagination accepted", err)
	}
	if store.calls.Load() != 1 {
		t.Fatal("invalid request reached repository")
	}
}
