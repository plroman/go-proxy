package proxy

// TODO: deduplicate requests
// TODO: use network server and user server namings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flashbots/go-utils/rpcserver"
	"github.com/flashbots/go-utils/rpctypes"
)

const maxRequestBodySizeBytes = 30 * 1024 * 1024 // 30 MB, @configurable

const (
	FlashbotsPeerName = "flashbots"

	EthSendBundleMethod         = "eth_sendBundle"
	MevSendBundleMethod         = "mev_sendBundle"
	EthCancelBundleMethod       = "eth_cancelBundle"
	EthSendRawTransactionMethod = "eth_sendRawTransaction"
	BidSubsidiseBlockMethod     = "bid_subsidiseBlock"
)

var (
	errUnknownPeer          = errors.New("unknown peers can't send to the public address")
	errSubsidyWrongEndpoint = errors.New("subsidy can only be called on public method")
	errSubsidyWrongCaller   = errors.New("subsidy can only be called by Flashbots")

	apiNow = time.Now
)

func (prx *Proxy) PublicJSONRPCHandler() (*rpcserver.JSONRPCHandler, error) {
	handler, err := rpcserver.NewJSONRPCHandler(rpcserver.Methods{
		EthSendBundleMethod:         prx.EthSendBundlePublic,
		MevSendBundleMethod:         prx.MevSendBundlePublic,
		EthCancelBundleMethod:       prx.EthCancelBundlePublic,
		EthSendRawTransactionMethod: prx.EthSendRawTransactionPublic,
		BidSubsidiseBlockMethod:     prx.BidSubsidiseBlockPublic,
	},
		rpcserver.JSONRPCHandlerOpts{
			ServerName:                       "network_server",
			Log:                              prx.Log,
			MaxRequestBodySizeBytes:          maxRequestBodySizeBytes,
			VerifyRequestSignatureFromHeader: true,
		},
	)

	return handler, err
}

func (prx *Proxy) LocalJSONRPCHandler() (*rpcserver.JSONRPCHandler, error) {
	handler, err := rpcserver.NewJSONRPCHandler(rpcserver.Methods{
		EthSendBundleMethod:         prx.EthSendBundleLocal,
		MevSendBundleMethod:         prx.MevSendBundleLocal,
		EthCancelBundleMethod:       prx.EthCancelBundleLocal,
		EthSendRawTransactionMethod: prx.EthSendRawTransactionLocal,
		BidSubsidiseBlockMethod:     prx.BidSubsidiseBlockLocal,
	},
		rpcserver.JSONRPCHandlerOpts{
			ServerName:                       "user_server",
			Log:                              prx.Log,
			MaxRequestBodySizeBytes:          maxRequestBodySizeBytes,
			VerifyRequestSignatureFromHeader: true,
		},
	)

	return handler, err
}

// IsValidPublicSigner verifies if signer is a valid peer and returns peer name
func (prx *Proxy) IsValidPublicSigner(address common.Address) (bool, string) {
	if address == prx.FlashbotsSignerAddress {
		return true, FlashbotsPeerName
	}
	prx.peersMu.RLock()
	found := false
	peerName := ""
	for _, peer := range prx.lastFetchedPeers {
		if address == peer.OrderflowProxy.EcdsaPubkeyAddress {
			found = true
			peerName = peer.Name
			break
		}
	}
	prx.peersMu.RUnlock()
	return found, peerName
}

func (prx *Proxy) EthSendBundle(ctx context.Context, ethSendBundle rpctypes.EthSendBundleArgs, publicEndpoint bool) error {
	err := ValidateEthSendBundle(&ethSendBundle, publicEndpoint)
	if err != nil {
		return err
	}
	signer := rpcserver.GetSigner(ctx)
	var peerName string
	if publicEndpoint {
		ok, name := prx.IsValidPublicSigner(signer)
		if !ok {
			return errUnknownPeer
		}
		peerName = name
	} else {
		ethSendBundle.SigningAddress = &signer
	}
	parsedRequest := ParsedRequest{
		publicEndpoint: publicEndpoint,
		signer:         signer,
		ethSendBundle:  &ethSendBundle,
		method:         EthSendBundleMethod,
		peerName:       peerName,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *Proxy) EthSendBundlePublic(ctx context.Context, ethSendBundle rpctypes.EthSendBundleArgs) error {
	return prx.EthSendBundle(ctx, ethSendBundle, true)
}

func (prx *Proxy) EthSendBundleLocal(ctx context.Context, ethSendBundle rpctypes.EthSendBundleArgs) error {
	return prx.EthSendBundle(ctx, ethSendBundle, false)
}

func (prx *Proxy) MevSendBundle(ctx context.Context, mevSendBundle rpctypes.MevSendBundleArgs, publicEndpoint bool) error {
	// TODO: make sure that cancellations are handled by the builder properly
	err := ValidateMevSendBundle(&mevSendBundle, publicEndpoint)
	if err != nil {
		return err
	}
	signer := rpcserver.GetSigner(ctx)
	var peerName string
	if publicEndpoint {
		ok, name := prx.IsValidPublicSigner(signer)
		if !ok {
			return errUnknownPeer
		}
		peerName = name
	} else {
		mevSendBundle.Metadata = &rpctypes.MevBundleMetadata{
			Signer: &signer,
		}
	}
	parsedRequest := ParsedRequest{
		publicEndpoint: publicEndpoint,
		signer:         signer,
		mevSendBundle:  &mevSendBundle,
		method:         MevSendBundleMethod,
		peerName:       peerName,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *Proxy) MevSendBundlePublic(ctx context.Context, mevSendBundle rpctypes.MevSendBundleArgs) error {
	return prx.MevSendBundle(ctx, mevSendBundle, true)
}

func (prx *Proxy) MevSendBundleLocal(ctx context.Context, mevSendBundle rpctypes.MevSendBundleArgs) error {
	return prx.MevSendBundle(ctx, mevSendBundle, false)
}

func (prx *Proxy) EthCancelBundle(ctx context.Context, ethCancelBundle rpctypes.EthCancelBundleArgs, publicEndpoint bool) error {
	err := ValidateEthCancelBundle(&ethCancelBundle, publicEndpoint)
	if err != nil {
		return err
	}
	signer := rpcserver.GetSigner(ctx)
	var peerName string
	if publicEndpoint {
		ok, name := prx.IsValidPublicSigner(signer)
		if !ok {
			return errUnknownPeer
		}
		peerName = name
	} else {
		ethCancelBundle.SigningAddress = &signer
	}
	parsedRequest := ParsedRequest{
		publicEndpoint:  publicEndpoint,
		signer:          signer,
		ethCancelBundle: &ethCancelBundle,
		method:          EthCancelBundleMethod,
		peerName:        peerName,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *Proxy) EthCancelBundlePublic(ctx context.Context, ethCancelBundle rpctypes.EthCancelBundleArgs) error {
	return prx.EthCancelBundle(ctx, ethCancelBundle, true)
}

func (prx *Proxy) EthCancelBundleLocal(ctx context.Context, ethCancelBundle rpctypes.EthCancelBundleArgs) error {
	return prx.EthCancelBundle(ctx, ethCancelBundle, false)
}

func (prx *Proxy) EthSendRawTransaction(ctx context.Context, ethSendRawTransaction rpctypes.EthSendRawTransactionArgs, publicEndpoint bool) error {
	signer := rpcserver.GetSigner(ctx)
	var peerName string
	if publicEndpoint {
		ok, name := prx.IsValidPublicSigner(signer)
		if !ok {
			return errUnknownPeer
		}
		peerName = name
	}
	parsedRequest := ParsedRequest{
		publicEndpoint:        publicEndpoint,
		signer:                signer,
		ethSendRawTransaction: &ethSendRawTransaction,
		method:                EthSendRawTransactionMethod,
		peerName:              peerName,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *Proxy) EthSendRawTransactionPublic(ctx context.Context, ethSendRawTransaction rpctypes.EthSendRawTransactionArgs) error {
	return prx.EthSendRawTransaction(ctx, ethSendRawTransaction, true)
}

func (prx *Proxy) EthSendRawTransactionLocal(ctx context.Context, ethSendRawTransaction rpctypes.EthSendRawTransactionArgs) error {
	return prx.EthSendRawTransaction(ctx, ethSendRawTransaction, false)
}

func (prx *Proxy) BidSubsidiseBlock(ctx context.Context, bidSubsidiseBlock rpctypes.BidSubsisideBlockArgs, publicEndpoint bool) error {
	signer := rpcserver.GetSigner(ctx)
	if publicEndpoint {
		if signer != prx.FlashbotsSignerAddress {
			return errSubsidyWrongCaller
		}
	} else {
		return errSubsidyWrongEndpoint
	}
	parsedRequest := ParsedRequest{
		publicEndpoint:    publicEndpoint,
		signer:            signer,
		bidSubsidiseBlock: &bidSubsidiseBlock,
		method:            BidSubsidiseBlockMethod,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *Proxy) BidSubsidiseBlockPublic(ctx context.Context, bidSubsidiseBlock rpctypes.BidSubsisideBlockArgs) error {
	return prx.BidSubsidiseBlock(ctx, bidSubsidiseBlock, true)
}

func (prx *Proxy) BidSubsidiseBlockLocal(ctx context.Context, bidSubsidiseBlock rpctypes.BidSubsisideBlockArgs) error {
	return prx.BidSubsidiseBlock(ctx, bidSubsidiseBlock, false)
}

type ParsedRequest struct {
	publicEndpoint        bool
	signer                common.Address
	method                string
	peerName              string
	receivedAt            time.Time
	ethSendBundle         *rpctypes.EthSendBundleArgs
	mevSendBundle         *rpctypes.MevSendBundleArgs
	ethCancelBundle       *rpctypes.EthCancelBundleArgs
	ethSendRawTransaction *rpctypes.EthSendRawTransactionArgs
	bidSubsidiseBlock     *rpctypes.BidSubsisideBlockArgs
}

func (prx *Proxy) HandleParsedRequest(ctx context.Context, parsedRequest ParsedRequest) error {
	parsedRequest.receivedAt = apiNow()
	prx.Log.Info("Received request", slog.Bool("isNetworkEndpoint", parsedRequest.publicEndpoint), slog.String("method", parsedRequest.method))
	if parsedRequest.publicEndpoint {
		incAPIIncomingRequestsByPeer(parsedRequest.peerName)
	}
	select {
	case <-ctx.Done():
	case prx.shareQueue <- &parsedRequest:
	}
	if !parsedRequest.publicEndpoint {
		select {
		case <-ctx.Done():
		case prx.archiveQueue <- &parsedRequest:
		}
	}
	return nil
}
