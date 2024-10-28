package proxy

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flashbots/go-utils/rpcserver"
	"github.com/flashbots/go-utils/rpctypes"
)

const maxRequestBodySizeBytes = 30 * 1024 * 1024 // 30 MB, @configurable

const (
	SendBundleMethod = "eth_sendBundle"
)

func (prx *NewProxy) PublicJSONRPCHandler() (*rpcserver.JSONRPCHandler, error) {
	handler, err := rpcserver.NewJSONRPCHandler(rpcserver.Methods{
		SendBundleMethod: prx.SendBundlePublic,
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
		SendBundleMethod: prx.SendBundleLocal,
	},
		rpcserver.JSONRPCHandlerOpts{
			Log:                              prx.Log,
			MaxRequestBodySizeBytes:          maxRequestBodySizeBytes,
			VerifyRequestSignatureFromHeader: true,
		},
	)

	return handler, err
}

func (prx *NewProxy) SendBundlePublic(ctx context.Context, ethSendBundle rpctypes.EthSendBundleArgs) error {
	signer := rpcserver.GetSigner(ctx)
	// signer must be one of the peers
	// TODO: validate args
	parsedRequest := ParsedRequest{
		publicEndpoint: true,
		signer:         signer,
		ethSendBundle:  &ethSendBundle,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

func (prx *NewProxy) SendBundleLocal(ctx context.Context, ethSendBundle rpctypes.EthSendBundleArgs) error {
	signer := rpcserver.GetSigner(ctx)
	// TODO: validate args
	ethSendBundle.SigningAddress = &signer
	parsedRequest := ParsedRequest{
		publicEndpoint: false,
		signer:         signer,
		ethSendBundle:  &ethSendBundle,
	}
	return prx.HandleParsedRequest(ctx, parsedRequest)
}

type ParsedRequest struct {
	publicEndpoint        bool
	signer                common.Address
	ethSendBundle         *rpctypes.EthSendBundleArgs
	mevSendBundle         *rpctypes.MevSendBundleArgs
	ethCancelBundle       *rpctypes.EthCancelBundleArgs
	ethSendRawTransaction *rpctypes.EthSendRawTransactionArgs
	bidSubsidiseBlock     *rpctypes.BidSubsisideBlockArgs
}

func (prx *NewProxy) HandleParsedRequest(ctx context.Context, parsedRequest ParsedRequest) error {
	// share with others
	// @bug can this block?
	prx.shareQueue <- &parsedRequest
	// prx.archiveQueue <- &parsedRequest
	return nil
}

// func (prx *Proxy) HandleRequest(req JSONRPCRequest, signer common.Address, publicEndpoint bool) *JSONRPCError {
// 	parsedRequest := ParsedRequest{
// 		publicEndpoint: publicEndpoint,
// 		signer:         signer,
// 	}
// 	switch req.Method {
// 	case "eth_sendBundle":
// 		ethSendBundle, err := parseSingleParam[EthSendBundleArgs](req.Params)
// 		if err != nil {
// 			return err
// 		}
// 		parsedRequest.ethSendBundle = ethSendBundle
// 	case "mev_sendBundle":
// 		mevSendBundle, err := parseSingleParam[MevSendBundleArgs](req.Params)
// 		if err != nil {
// 			return err
// 		}
// 		parsedRequest.mevSendBundle = mevSendBundle
// 	case "eth_cancelBundle":
// 		ethCancelBundle, err := parseSingleParam[EthCancelBundleArgs](req.Params)
// 		if err != nil {
// 			return err
// 		}
// 		parsedRequest.ethCancelBundle = ethCancelBundle
// 	case "eth_sendRawTransaction":
// 		ethSendRawTransaction, err := parseSingleParam[EthSendRawTransactionArgs](req.Params)
// 		if err != nil {
// 			return err
// 		}
// 		parsedRequest.ethSendRawTransaction = ethSendRawTransaction
// 	case "bid_subsidiseBlock":
// 		bidSubsidiseBlock, err := parseSingleParam[BidSubsisideBlockArgs](req.Params)
// 		if err != nil {
// 			return err
// 		}
// 		parsedRequest.bidSubsidiseBlock = bidSubsidiseBlock
// 	default:
// 		return &errMethodNotFound
// 	}
// 	return prx.HandleParsedRequest(parsedRequest)
// }
