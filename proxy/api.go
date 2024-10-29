package proxy

import (
	"context"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flashbots/go-utils/rpcserver"
	"github.com/flashbots/go-utils/rpctypes"
)

const maxRequestBodySizeBytes = 30 * 1024 * 1024 // 30 MB, @configurable

const (
	EthSendBundleMethod         = "eth_sendBundle"
	MevSendBundleMethod         = "mev_sendBundle"
	EthCancelBundleMethod       = "eth_cancelBundle"
	EthSendRawTransactionMethod = "eth_sendRawTransaction"
	BidSubsidiseBlockMethod     = "bid_subsidiseBlock"
)

var errUnknownPeer = errors.New("unknown peers can't send to the public address")

func (prx *NewProxy) PublicJSONRPCHandler() (*rpcserver.JSONRPCHandler, error) {
	handler, err := rpcserver.NewJSONRPCHandler(rpcserver.Methods{
		EthSendBundleMethod:         prx.EthSendBundlePublic,
		MevSendBundleMethod:         prx.MevSendBundlePublic,
		EthCancelBundleMethod:       prx.EthCancelBundlePublic,
		EthSendRawTransactionMethod: prx.EthSendRawTransactionPublic,
		BidSubsidiseBlockMethod:     prx.BidSubsidiseBlockPublic,
	},
		rpcserver.JSONRPCHandlerOpts{
			Log:                              prx.Log,
			MaxRequestBodySizeBytes:          maxRequestBodySizeBytes,
			VerifyRequestSignatureFromHeader: true,
		},
	)

	return handler, err
}

func (prx *NewProxy) LocalJSONRPCHandler() (*rpcserver.JSONRPCHandler, error) {
	handler, err := rpcserver.NewJSONRPCHandler(rpcserver.Methods{
		EthSendBundleMethod:         prx.EthSendBundleLocal,
		MevSendBundleMethod:         prx.MevSendBundleLocal,
		EthCancelBundleMethod:       prx.EthCancelBundleLocal,
		EthSendRawTransactionMethod: prx.EthSendRawTransactionLocal,
		BidSubsidiseBlockMethod:     prx.BidSubsidiseBlockLocal,
	},
		rpcserver.JSONRPCHandlerOpts{
			Log:                              prx.Log,
			MaxRequestBodySizeBytes:          maxRequestBodySizeBytes,
			VerifyRequestSignatureFromHeader: true,
		},
	)

	return handler, err
}

// IsValidPublicSigner verifies if signer is a valid peer
func (prx *NewProxy) IsValidPublicSigner(address common.Address) bool {
	if address == prx.FlashbotsSignerAddress {
		return true
	}
	prx.peersMu.RLock()
	found := false
	for _, peer := range prx.lastFetchedPeers {
		if address == peer.OrderflowProxy.EcdsaPubkeyAddress {
			found = true
			break
		}
	}
	prx.peersMu.RUnlock()
	return found
}

func (prx *NewProxy) EthSendBundle(ctx context.Context, ethSendBundle rpctypes.EthSendBundleArgs, publicEndpoint bool) error {
	// TODO: validate args
	signer := rpcserver.GetSigner(ctx)
	if publicEndpoint {
		if !prx.IsValidPublicSigner(signer) {
			return errUnknownPeer
		}
	} else {
		ethSendBundle.SigningAddress = &signer
	}
	parsedRequest := ParsedRequest{
		publicEndpoint: publicEndpoint,
		signer:         signer,
		ethSendBundle:  &ethSendBundle,
		method:         EthSendBundleMethod,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *NewProxy) EthSendBundlePublic(ctx context.Context, ethSendBundle rpctypes.EthSendBundleArgs) error {
	return prx.EthSendBundle(ctx, ethSendBundle, true)
}

func (prx *NewProxy) EthSendBundleLocal(ctx context.Context, ethSendBundle rpctypes.EthSendBundleArgs) error {
	return prx.EthSendBundle(ctx, ethSendBundle, false)
}

func (prx *NewProxy) MevSendBundle(ctx context.Context, mevSendBundle rpctypes.MevSendBundleArgs, publicEndpoint bool) error {
	// TODO: validate args, handle cancellations
	signer := rpcserver.GetSigner(ctx)
	if publicEndpoint {
		if !prx.IsValidPublicSigner(signer) {
			return errUnknownPeer
		}
	} else {
		mevSendBundle.Metadata.Signer = &signer
	}
	parsedRequest := ParsedRequest{
		publicEndpoint: publicEndpoint,
		signer:         signer,
		mevSendBundle:  &mevSendBundle,
		method:         MevSendBundleMethod,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *NewProxy) MevSendBundlePublic(ctx context.Context, mevSendBundle rpctypes.MevSendBundleArgs) error {
	return prx.MevSendBundle(ctx, mevSendBundle, true)
}

func (prx *NewProxy) MevSendBundleLocal(ctx context.Context, mevSendBundle rpctypes.MevSendBundleArgs) error {
	return prx.MevSendBundle(ctx, mevSendBundle, false)
}

func (prx *NewProxy) EthCancelBundle(ctx context.Context, ethCancelBundle rpctypes.EthCancelBundleArgs, publicEndpoint bool) error {
	// TODO: validate args
	signer := rpcserver.GetSigner(ctx)
	if publicEndpoint {
		if !prx.IsValidPublicSigner(signer) {
			return errUnknownPeer
		}
	} else {
		ethCancelBundle.SigningAddress = &signer
	}
	parsedRequest := ParsedRequest{
		publicEndpoint:  publicEndpoint,
		signer:          signer,
		ethCancelBundle: &ethCancelBundle,
		method:          EthCancelBundleMethod,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *NewProxy) EthCancelBundlePublic(ctx context.Context, ethCancelBundle rpctypes.EthCancelBundleArgs) error {
	return prx.EthCancelBundle(ctx, ethCancelBundle, true)
}

func (prx *NewProxy) EthCancelBundleLocal(ctx context.Context, ethCancelBundle rpctypes.EthCancelBundleArgs) error {
	return prx.EthCancelBundle(ctx, ethCancelBundle, false)
}

func (prx *NewProxy) EthSendRawTransaction(ctx context.Context, ethSendRawTransaction rpctypes.EthSendRawTransactionArgs, publicEndpoint bool) error {
	// TODO: convert to mev send bundle when sharing
	signer := rpcserver.GetSigner(ctx)
	if publicEndpoint {
		if !prx.IsValidPublicSigner(signer) {
			return errUnknownPeer
		}
	}
	parsedRequest := ParsedRequest{
		publicEndpoint:        publicEndpoint,
		signer:                signer,
		ethSendRawTransaction: &ethSendRawTransaction,
		method:                EthSendRawTransactionMethod,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *NewProxy) EthSendRawTransactionPublic(ctx context.Context, ethSendRawTransaction rpctypes.EthSendRawTransactionArgs) error {
	return prx.EthSendRawTransaction(ctx, ethSendRawTransaction, true)
}

func (prx *NewProxy) EthSendRawTransactionLocal(ctx context.Context, ethSendRawTransaction rpctypes.EthSendRawTransactionArgs) error {
	return prx.EthSendRawTransaction(ctx, ethSendRawTransaction, false)
}

func (prx *NewProxy) BidSubsidiseBlock(ctx context.Context, bidSubsidiseBlock rpctypes.BidSubsisideBlockArgs, publicEndpoint bool) error {
	signer := rpcserver.GetSigner(ctx)
	if publicEndpoint {
		if signer != prx.FlashbotsSignerAddress {
			return errUnknownPeer
		}
	}
	parsedRequest := ParsedRequest{
		publicEndpoint:    publicEndpoint,
		signer:            signer,
		bidSubsidiseBlock: &bidSubsidiseBlock,
		method:            BidSubsidiseBlockMethod,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *NewProxy) BidSubsidiseBlockPublic(ctx context.Context, bidSubsidiseBlock rpctypes.BidSubsisideBlockArgs) error {
	return prx.BidSubsidiseBlock(ctx, bidSubsidiseBlock, true)
}

func (prx *NewProxy) BidSubsidiseBlockLocal(ctx context.Context, bidSubsidiseBlock rpctypes.BidSubsisideBlockArgs) error {
	return prx.BidSubsidiseBlock(ctx, bidSubsidiseBlock, false)
}

type ParsedRequest struct {
	publicEndpoint        bool
	signer                common.Address
	method                string
	receivedAt            time.Time
	ethSendBundle         *rpctypes.EthSendBundleArgs
	mevSendBundle         *rpctypes.MevSendBundleArgs
	ethCancelBundle       *rpctypes.EthCancelBundleArgs
	ethSendRawTransaction *rpctypes.EthSendRawTransactionArgs
	bidSubsidiseBlock     *rpctypes.BidSubsisideBlockArgs
}

func (prx *NewProxy) HandleParsedRequest(ctx context.Context, parsedRequest ParsedRequest) error {
	parsedRequest.receivedAt = time.Now()
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
