package proxy

// TODO: use multiple connections / senders per receiver

import (
	"context"
	"log/slog"
	"time"

	"github.com/flashbots/go-utils/rpcclient"
	"github.com/flashbots/go-utils/signature"
)

var (
	jobBufferSize  = 4096
	requestTimeout = time.Second * 10
)

type ShareQueue struct {
	name         string
	log          *slog.Logger
	queue        chan *ParsedRequest
	updatePeers  chan []ConfighubBuilder
	localBuilder rpcclient.RPCClient
	singer       *signature.Signer
}

func (sq *ShareQueue) Run() {
	var (
		localBuilder = make(chan *ParsedRequest)
		peers        []chan *ParsedRequest
	)
	defer close(localBuilder)
	go sq.proxyRequests(localBuilder, sq.localBuilder, "local-builder")
	for {
		select {
		case req, more := <-sq.queue:
			sq.log.Debug("Received request", slog.String("name", sq.name), slog.String("method", req.method))
			if !more {
				return
			}
			select {
			case localBuilder <- req:
			default:
				// @log
			}
			if !req.publicEndpoint {
				for _, peer := range peers {
					select {
					case peer <- req:
					default:
						// @log
					}
				}
			}
		case newPeers, more := <-sq.updatePeers:
			if !more {
				return
			}
			for _, peer := range peers {
				close(peer)
			}
			peers = nil
			for _, info := range newPeers {
				// don't send to yourself
				if info.Name == sq.name {
					continue
				}
				client, err := RPCClientWithCertAndSigner(OrderflowProxyURLFromIP(info.IP), []byte(info.OrderflowProxy.TLSCert), sq.singer)
				if err != nil {
					sq.log.Error("Failed to create a peer client", slog.Any("error", err))
					continue
				}
				sq.log.Info("Created client for peer", slog.String("peer", info.Name), slog.String("name", sq.name))
				ch := make(chan *ParsedRequest, jobBufferSize)
				peers = append(peers, ch)
				go sq.proxyRequests(ch, client, info.Name)
			}
		}
	}
}

func (sq *ShareQueue) proxyRequests(ch chan *ParsedRequest, client rpcclient.RPCClient, name string) {
	logger := sq.log.With(slog.String("target", name), slog.String("name", sq.name))
	for {
		req, more := <-ch
		if !more {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		var (
			method string
			data   any
		)
		if req.ethSendBundle != nil {
			method = EthSendBundleMethod
			data = req.ethSendBundle
		} else if req.mevSendBundle != nil {
			method = MevSendBundleMethod
			data = req.mevSendBundle
		} else if req.ethCancelBundle != nil {
			method = EthCancelBundleMethod
			data = req.ethCancelBundle
		} else if req.ethSendRawTransaction != nil {
			method = EthSendRawTransactionMethod
			data = req.ethSendRawTransaction
		} else if req.bidSubsidiseBlock != nil {
			continue
		} else {
			logger.Error("Unknown request type", slog.String("name", sq.name))
			continue
		}
		resp, err := client.Call(ctx, method, data)
		if err != nil {
			logger.Warn("Error while proxying request", slog.Any("error", err))
		}
		if resp != nil && resp.Error != nil {
			logger.Warn("Error returned form target while proxying", slog.Any("error", resp.Error))
		}
		logger.Debug("Message proxied")
	}
}
