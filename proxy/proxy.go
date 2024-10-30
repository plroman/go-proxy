package proxy

import (
	"crypto/tls"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flashbots/go-utils/rpcclient"
	"github.com/flashbots/go-utils/signature"
	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

var (
	requestsRLUSize = 4096
	requestsRLUTTL  = time.Second * 12

	peerUpdateTime = time.Minute * 5
)

type Proxy struct {
	NewProxyConstantConfig

	ConfigHub *BuilderConfigHub

	OrderflowSigner *signature.Signer
	PublicCertPEM   []byte
	Certificate     tls.Certificate

	localBuilder rpcclient.RPCClient

	PublicHandler http.Handler
	LocalHandler  http.Handler
	CertHandler   http.Handler // this endpoint just returns generated certificate

	updatePeers chan []ConfighubBuilder
	shareQueue  chan *ParsedRequest

	archiveQueue      chan *ParsedRequest
	archiveFlushQueue chan struct{}

	peersMu          sync.RWMutex
	lastFetchedPeers []ConfighubBuilder

	requestUniqueKeysRLU *expirable.LRU[uuid.UUID, struct{}]

	peerUpdaterClose chan struct{}
}

type NewProxyConstantConfig struct {
	Log                    *slog.Logger
	Name                   string
	FlashbotsSignerAddress common.Address
}

type NewProxyConfig struct {
	NewProxyConstantConfig
	CertValidDuration time.Duration
	CertHosts         []string

	BuilderConfigHubEndpoint string
	ArchiveEndpoint          string
	LocalBuilderEndpoint     string

	// EthRPC should support eth_blockNumber API
	EthRPC string
}

func NewNewProxy(config NewProxyConfig) (*Proxy, error) {
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

	prx := &Proxy{
		NewProxyConstantConfig: config.NewProxyConstantConfig,
		ConfigHub:              NewBuilderConfigHub(config.Log, config.BuilderConfigHubEndpoint),
		OrderflowSigner:        orderflowSigner,
		PublicCertPEM:          cert,
		Certificate:            certificate,
		localBuilder:           localBuilder,
		requestUniqueKeysRLU:   expirable.NewLRU[uuid.UUID, struct{}](requestsRLUSize, nil, requestsRLUTTL),
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
		if err != nil {
			prx.Log.Warn("Failed to serve certificate", slog.Any("error", err))
		}
	})

	shareQeueuCh := make(chan *ParsedRequest)
	updatePeersCh := make(chan []ConfighubBuilder)
	prx.shareQueue = shareQeueuCh
	prx.updatePeers = updatePeersCh
	queue := ShareQueue{
		name:         prx.Name,
		log:          prx.Log,
		queue:        shareQeueuCh,
		updatePeers:  updatePeersCh,
		localBuilder: prx.localBuilder,
		singer:       prx.OrderflowSigner,
	}
	go queue.Run()

	archiveQueueCh := make(chan *ParsedRequest)
	archiveFlushCh := make(chan struct{})
	prx.archiveQueue = archiveQueueCh
	prx.archiveFlushQueue = archiveFlushCh
	archiveClient := rpcclient.NewClientWithOpts(config.ArchiveEndpoint, &rpcclient.RPCClientOpts{
		Signer: orderflowSigner,
	})
	archiveQueue := ArchiveQueue{
		log:               prx.Log,
		queue:             archiveQueueCh,
		flushQueue:        archiveFlushCh,
		archiveClient:     archiveClient,
		blockNumberSource: NewBlockNumberSource(config.EthRPC),
	}
	go archiveQueue.Run()

	prx.peerUpdaterClose = make(chan struct{})
	go func() {
		for {
			select {
			case _, more := <-prx.peerUpdaterClose:
				if !more {
					return
				}
			case <-time.After(peerUpdateTime):
				err := prx.RequestNewPeers()
				if err != nil {
					prx.Log.Error("Failed to update peers", slog.Any("error", err))
				}
			}
		}
	}()

	return prx, nil
}

func (prx *Proxy) Stop() {
	close(prx.shareQueue)
	close(prx.updatePeers)
	close(prx.archiveQueue)
	close(prx.archiveFlushQueue)
	close(prx.peerUpdaterClose)
}

func (prx *Proxy) TLSConfig() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{prx.Certificate},
		MinVersion:   tls.VersionTLS13,
	}
}

func (prx *Proxy) RegisterSecrets() error {
	// TODO: add retries
	return prx.ConfigHub.RegisterCredentials(ConfighubOrderflowProxyCredentials{
		TLSCert:            string(prx.PublicCertPEM),
		EcdsaPubkeyAddress: prx.OrderflowSigner.Address(),
	})
}

// RequestNewPeers updates currently available peers from the builder config hub
func (prx *Proxy) RequestNewPeers() error {
	builders, err := prx.ConfigHub.Builders()
	if err != nil {
		return err
	}

	prx.peersMu.Lock()
	prx.lastFetchedPeers = builders
	prx.peersMu.Unlock()

	select {
	case prx.updatePeers <- builders:
	default:
	}
	return nil
}

// FlushArchiveQueue forces the archive queue to flush
func (prx *Proxy) FlushArchiveQueue() {
	prx.archiveFlushQueue <- struct{}{}
}
