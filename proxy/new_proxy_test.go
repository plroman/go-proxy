package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/flashbots/go-utils/rpctypes"
	"github.com/flashbots/go-utils/signature"
	"github.com/stretchr/testify/require"
)

type RequestData struct {
	request *http.Request
	body    string
}

var (
	builderHub *httptest.Server

	builderHubPeers []ConfighubBuilder

	archiveServer         *httptest.Server
	archiveServerRequests chan *RequestData

	proxies []*OrderflowProxyTestSetup
)

func ServeHTTPRequestToChan(channel chan *RequestData) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		channel <- &RequestData{body: string(body), request: r}
		w.WriteHeader(http.StatusOK)
	}))
}

type OrderflowProxyTestSetup struct {
	proxy        *NewProxy
	publicServer *http.Server
	localServer  *http.Server
	certServer   *httptest.Server

	ip                   string
	publicServerEndpoint string
	localServerEndpoint  string

	localBuilderServer   *httptest.Server
	localBuilderRequests chan *RequestData
}

func (setup *OrderflowProxyTestSetup) Close() {
	_ = setup.publicServer.Close()
	_ = setup.localServer.Close()
	setup.certServer.Close()
	setup.localBuilderServer.Close()
}

func StartTestOrderflowProxy(name string) (*OrderflowProxyTestSetup, error) {
	localBuilderRequests := make(chan *RequestData, 1)
	localBuilderServer := ServeHTTPRequestToChan(localBuilderRequests)

	proxy := createProxy(localBuilderServer.URL, name)
	publicProxyServer := &http.Server{
		Handler:   proxy.PublicHandler,
		TLSConfig: proxy.TLSConfig(),
	}
	publicListener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	go publicProxyServer.ServeTLS(publicListener, "", "")
	publicServerEndpoint := fmt.Sprintf("https://localhost:%d", publicListener.Addr().(*net.TCPAddr).Port)
	ip := fmt.Sprintf("127.0.0.1:%d", publicListener.Addr().(*net.TCPAddr).Port)

	localProxyServer := &http.Server{
		Handler:   proxy.LocalHandler,
		TLSConfig: proxy.TLSConfig(),
	}
	localListener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	go localProxyServer.ServeTLS(localListener, "", "")
	localServerEndpoint := fmt.Sprintf("https://localhost:%d", localListener.Addr().(*net.TCPAddr).Port)

	certProxyServer := httptest.NewServer(proxy.CertHandler)

	return &OrderflowProxyTestSetup{
		proxy:                proxy,
		publicServer:         publicProxyServer,
		localServer:          localProxyServer,
		certServer:           certProxyServer,
		publicServerEndpoint: publicServerEndpoint,
		localServerEndpoint:  localServerEndpoint,
		localBuilderServer:   localBuilderServer,
		localBuilderRequests: localBuilderRequests,
		ip:                   ip,
	}, nil
}

func TestMain(m *testing.M) {
	archiveServerRequests = make(chan *RequestData, 1)
	builderHub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		if r.URL.Path == "/api/l1-builder/v1/register_credentials/orderflow-proxy" {
			var req ConfighubOrderflowProxyCredentials
			err := json.Unmarshal(body, &req)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(err.Error()))
			}
			var (
				ip   string
				name string
			)
			for _, proxy := range proxies {
				if proxy.proxy.OrderflowSigner.Address() == req.EcdsaPubkeyAddress {
					ip = proxy.ip
					name = proxy.proxy.Name
					break
				}
			}
			builderHubPeers = append(builderHubPeers, ConfighubBuilder{
				Name:           name,
				IP:             ip,
				OrderflowProxy: req,
			})
		} else if r.URL.Path == "/api/l1-builder/v1/builders" {
			res, err := json.Marshal(builderHubPeers)
			if err != nil {
				panic(err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(res)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer builderHub.Close()

	archiveServer = ServeHTTPRequestToChan(archiveServerRequests)
	defer archiveServer.Close()

	for i := 0; i < 3; i++ {
		proxy, err := StartTestOrderflowProxy(fmt.Sprintf("proxy:%d", i))
		proxies = append(proxies, proxy)
		if err != nil {
			panic(err)
		}
	}
	defer func() {
		for _, prx := range proxies {
			prx.Close()
		}
	}()

	os.Exit(m.Run())
}

func createProxy(localBuilder, name string) *NewProxy {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	proxy, err := NewNewProxy(NewProxyConfig{
		NewProxyConstantConfig: NewProxyConstantConfig{
			Log:  log,
			Name: name,
		},
		CertValidDuration:        time.Hour * 24,
		CertHosts:                []string{"localhost", "127.0.0.1"},
		BuilderConfigHubEndpoint: builderHub.URL,
		LocalBuilderEndpoint:     localBuilder,
	})
	if err != nil {
		panic(err)
	}
	return proxy
}

func TestPublishSecrets(t *testing.T) {
	err := proxies[0].proxy.RegisterSecrets()
	require.NoError(t, err)
}

func TestServeCert(t *testing.T) {
	resp, err := http.Get(proxies[0].certServer.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, string(proxies[0].proxy.PublicCertPEM), string(body))
}

func expectRequest(t *testing.T, ch chan *RequestData) *RequestData {
	t.Helper()
	select {
	case req := <-ch:
		return req
	case <-time.After(time.Millisecond * 100):
		t.Fatal("Timeout while waiting for request")
		return nil
	}
}

func expectNoRequest(t *testing.T, ch chan *RequestData) {
	t.Helper()
	select {
	case req := <-ch:
		t.Fatal("Unexpected request", req)
	case <-time.After(time.Millisecond * 100):
	}
}

func TestProxyBundleRequestWithPeerUpdate(t *testing.T) {
	signer, err := signature.NewSignerFromHexPrivateKey("0xd63b3c447fdea415a05e4c0b859474d14105a88178efdf350bc9f7b05be3cc58")
	require.NoError(t, err)
	client, err := RPCClientWithCertAndSigner(proxies[0].localServerEndpoint, proxies[0].proxy.PublicCertPEM, signer)
	require.NoError(t, err)

	expectedRequest := `{"method":"eth_sendBundle","params":[{"txs":null,"blockNumber":"0x3e8","signingAddress":"0x9349365494be4f6205e5d44bdc7ec7dcd134becf"}],"id":0,"jsonrpc":"2.0"}`

	// we start with no peers
	builderHubPeers = nil
	err = proxies[0].proxy.RegisterSecrets()
	require.NoError(t, err)
	err = proxies[0].proxy.RequestNewPeers()
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 50)

	_, err = client.Call(context.Background(), SendBundleMethod, &rpctypes.EthSendBundleArgs{
		BlockNumber: 1000,
	})
	require.NoError(t, err)

	builderRequest := expectRequest(t, proxies[0].localBuilderRequests)
	require.Equal(t, expectedRequest, builderRequest.body)
	expectNoRequest(t, proxies[1].localBuilderRequests)
	expectNoRequest(t, proxies[2].localBuilderRequests)

	// add one more peer
	err = proxies[1].proxy.RegisterSecrets()
	require.NoError(t, err)

	err = proxies[0].proxy.RequestNewPeers()
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 50)

	_, err = client.Call(context.Background(), SendBundleMethod, &rpctypes.EthSendBundleArgs{
		BlockNumber: 1000,
	})
	require.NoError(t, err)

	builderRequest = expectRequest(t, proxies[0].localBuilderRequests)
	require.Equal(t, expectedRequest, builderRequest.body)
	builderRequest = expectRequest(t, proxies[1].localBuilderRequests)
	require.Equal(t, expectedRequest, builderRequest.body)
	expectNoRequest(t, proxies[2].localBuilderRequests)

	// add another peer
	err = proxies[2].proxy.RegisterSecrets()
	require.NoError(t, err)

	err = proxies[0].proxy.RequestNewPeers()
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 50)

	_, err = client.Call(context.Background(), SendBundleMethod, &rpctypes.EthSendBundleArgs{
		BlockNumber: 1000,
	})
	require.NoError(t, err)

	builderRequest = expectRequest(t, proxies[0].localBuilderRequests)
	require.Equal(t, expectedRequest, builderRequest.body)
	builderRequest = expectRequest(t, proxies[1].localBuilderRequests)
	require.Equal(t, expectedRequest, builderRequest.body)
	builderRequest = expectRequest(t, proxies[2].localBuilderRequests)
	require.Equal(t, expectedRequest, builderRequest.body)
}
