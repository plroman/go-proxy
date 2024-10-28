package proxy

import (
	"crypto/tls"
	"log/slog"
	"net/http"
	"time"

	"github.com/flashbots/go-utils/rpcclient"
	"github.com/flashbots/go-utils/signature"
)

type NewProxy struct {
	NewProxyConstantConfig

	ConfigHub *BuilderConfigHub

	OrderflowSigner *signature.Signer
	PublicCertPEM   []byte
	Certificate     tls.Certificate

	LocalBuilder rpcclient.RPCClient

	PublicHandler http.Handler
	LocalHandler  http.Handler
	CertHandler   http.Handler // this endpoint just returns generated certificate

	UpdatePeers chan []ConfighubBuilder
	shareQueue  chan *ParsedRequest
}

type NewProxyConstantConfig struct {
	Log  *slog.Logger
	Name string
}

type NewProxyConfig struct {
	NewProxyConstantConfig
	CertValidDuration time.Duration
	CertHosts         []string

	BuilderConfigHubEndpoint string

	LocalBuilderEndpoint string
}

func NewNewProxy(config NewProxyConfig) (*NewProxy, error) {
	orderflowSigner, err := signature.NewRandomSigner()
	if err != nil {
		return nil, err
	}
	cert, key, err := GenerateCert(config.CertValidDuration, config.CertHosts)
	if err != nil {
		return nil, err
	}

	certificate, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}

	localBuilder := rpcclient.NewClient(config.LocalBuilderEndpoint)

	prx := &NewProxy{
		NewProxyConstantConfig: config.NewProxyConstantConfig,
		ConfigHub:              NewBuilderConfigHub(config.BuilderConfigHubEndpoint),
		OrderflowSigner:        orderflowSigner,
		PublicCertPEM:          cert,
		Certificate:            certificate,
		LocalBuilder:           localBuilder,
	}

	publicHandler, err := prx.PublicJSONRPCHandler()
	if err != nil {
		return nil, err
	}
	prx.PublicHandler = publicHandler

	localHandler, err := prx.LocalJSONRPCHandler()
	if err != nil {
		return nil, err
	}
	prx.LocalHandler = localHandler

	prx.CertHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/octet-stream")
		_, err := w.Write([]byte(prx.PublicCertPEM))
		prx.Log.Warn("Failed to serve certificate", slog.Any("error", err))
	})

	shareQeueuCh := make(chan *ParsedRequest)
	updatePeersCh := make(chan []ConfighubBuilder)
	prx.shareQueue = shareQeueuCh
	prx.UpdatePeers = updatePeersCh
	queue := ShareQueue{
		name:         prx.Name,
		log:          prx.Log,
		queue:        shareQeueuCh,
		updatePeers:  updatePeersCh,
		localBuilder: prx.LocalBuilder,
		singer:       prx.OrderflowSigner,
	}
	go queue.Run()

	return prx, nil
}

func (prx *NewProxy) TLSConfig() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{prx.Certificate},
		MinVersion:   tls.VersionTLS13,
	}
}

func (prx *NewProxy) RegisterSecrets() error {
	return prx.ConfigHub.RegisterCredentials(ConfighubOrderflowProxyCredentials{
		TLSCert:            string(prx.PublicCertPEM),
		EcdsaPubkeyAddress: prx.OrderflowSigner.Address(),
	})
}

// RequestNewPeers updates currently available peers from the builder config hub
func (prx *NewProxy) RequestNewPeers() error {
	builders, err := prx.ConfigHub.Builders()
	if err != nil {
		return err
	}
	select {
	case prx.UpdatePeers <- builders:
	default:
	}
	return nil
}
