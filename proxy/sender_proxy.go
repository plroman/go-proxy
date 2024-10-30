package proxy

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/flashbots/go-utils/rpcserver"
	"github.com/flashbots/go-utils/rpctypes"
	"github.com/flashbots/go-utils/signature"
)

type SenderProxyConstantConfig struct {
	Log             *slog.Logger
	OrderflowSigner *signature.Signer
}

type SenderProxyConfig struct {
	SenderProxyConstantConfig
	BuilderConfigHubEndpoint string
	MaxRequestBodySizeBytes  int64
}

type SenderProxy struct {
	SenderProxyConstantConfig
	ConfigHub *BuilderConfigHub
	Handler   http.Handler

	updatePeers chan []ConfighubBuilder
	shareQueue  chan *ParsedRequest

	peerUpdaterClose chan struct{}
}

func NewSenderProxy(config SenderProxyConfig) (*SenderProxy, error) {
	maxRequestBodySizeBytes := DefaultMaxRequestBodySizeBytes
	if config.MaxRequestBodySizeBytes != 0 {
		maxRequestBodySizeBytes = config.MaxRequestBodySizeBytes
	}

	prx := &SenderProxy{
		SenderProxyConstantConfig: config.SenderProxyConstantConfig,
		ConfigHub:                 NewBuilderConfigHub(config.Log, config.BuilderConfigHubEndpoint),
		Handler:                   nil,
		updatePeers:               make(chan []ConfighubBuilder),
		shareQueue:                make(chan *ParsedRequest),
		peerUpdaterClose:          make(chan struct{}),
	}

	handler, err := rpcserver.NewJSONRPCHandler(rpcserver.Methods{
		EthSendBundleMethod:         prx.EthSendBundle,
		MevSendBundleMethod:         prx.MevSendBundle,
		EthCancelBundleMethod:       prx.EthCancelBundle,
		EthSendRawTransactionMethod: prx.EthSendRawTransaction,
		BidSubsidiseBlockMethod:     prx.BidSubsidiseBlock,
	},
		rpcserver.JSONRPCHandlerOpts{
			Log:                     prx.Log,
			MaxRequestBodySizeBytes: maxRequestBodySizeBytes,
		},
	)
	if err != nil {
		return nil, err
	}
	prx.Handler = handler

	queue := ShareQueue{
		log:          prx.Log,
		queue:        prx.shareQueue,
		updatePeers:  prx.updatePeers,
		localBuilder: nil,
		signer:       prx.OrderflowSigner,
	}
	go queue.Run()

	go func() {
		for {
			select {
			case _, more := <-prx.peerUpdaterClose:
				if !more {
					return
				}
			case <-time.After(peerUpdateTime):
				builders, err := prx.ConfigHub.Builders()
				if err != nil {
					prx.Log.Error("Failed to update peers", slog.Any("error", err))
				}

				select {
				case prx.updatePeers <- builders:
				default:
				}
			}
		}
	}()
	return prx, nil
}

func (prx *SenderProxy) Stop() {
	close(prx.shareQueue)
	close(prx.updatePeers)
	close(prx.peerUpdaterClose)
}

func (prx *SenderProxy) EthSendBundle(ctx context.Context, ethSendBundle rpctypes.EthSendBundleArgs) error {
	parsedRequest := ParsedRequest{
		publicEndpoint: true,
		ethSendBundle:  &ethSendBundle,
		method:         EthSendBundleMethod,
	}

	err := ValidateEthSendBundle(&ethSendBundle, true)
	if err != nil {
		return err
	}

	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *SenderProxy) MevSendBundle(ctx context.Context, mevSendBundle rpctypes.MevSendBundleArgs) error {
	parsedRequest := ParsedRequest{
		publicEndpoint: true,
		mevSendBundle:  &mevSendBundle,
		method:         MevSendBundleMethod,
	}

	err := ValidateMevSendBundle(&mevSendBundle, true)
	if err != nil {
		return err
	}

	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *SenderProxy) EthCancelBundle(ctx context.Context, ethCancelBundle rpctypes.EthCancelBundleArgs) error {
	parsedRequest := ParsedRequest{
		publicEndpoint:  true,
		ethCancelBundle: &ethCancelBundle,
		method:          EthCancelBundleMethod,
	}

	err := ValidateEthCancelBundle(&ethCancelBundle, true)
	if err != nil {
		return err
	}

	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *SenderProxy) EthSendRawTransaction(ctx context.Context, ethSendRawTransaction rpctypes.EthSendRawTransactionArgs) error {
	parsedRequest := ParsedRequest{
		publicEndpoint:        true,
		ethSendRawTransaction: &ethSendRawTransaction,
		method:                EthSendRawTransactionMethod,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *SenderProxy) BidSubsidiseBlock(ctx context.Context, bidSubsidiseBlock rpctypes.BidSubsisideBlockArgs) error {
	parsedRequest := ParsedRequest{
		publicEndpoint:    true,
		bidSubsidiseBlock: &bidSubsidiseBlock,
		method:            BidSubsidiseBlockMethod,
	}

	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *SenderProxy) HandleParsedRequest(ctx context.Context, parsedRequest ParsedRequest) error {
	parsedRequest.receivedAt = apiNow()
	prx.Log.Info("Received request", slog.String("method", parsedRequest.method))

	select {
	case <-ctx.Done():
	case prx.shareQueue <- &parsedRequest:
	}
	return nil
}
