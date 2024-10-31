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
	signer       *signature.Signer
}

type shareQueuePeer struct {
	ch     chan *ParsedRequest
	name   string
	client rpcclient.RPCClient
}

func newShareQueuePeer(name string, client rpcclient.RPCClient) shareQueuePeer {
	return shareQueuePeer{
		ch:     make(chan *ParsedRequest, jobBufferSize),
		name:   name,
		client: client,
	}
}

func (p *shareQueuePeer) Close() {
	close(p.ch)
}

func (p *shareQueuePeer) SendRequest(log *slog.Logger, request *ParsedRequest) {
	select {
	case p.ch <- request:
	default:
		log.Error("Peer is stalling on requests", slog.String("peer", p.name))
		incShareQueuePeerStallingErrors(p.name)
	}
}

func (sq *ShareQueue) Run() {
	var (
		localBuilder *shareQueuePeer
		peers        []shareQueuePeer
	)
	if sq.localBuilder != nil {
		builderPeer := newShareQueuePeer("local-builder", sq.localBuilder)
		localBuilder = &builderPeer
		go sq.proxyRequests(localBuilder)
		defer localBuilder.Close()
	}
	for {
		select {
		case req, more := <-sq.queue:
			sq.log.Debug("Share queue received a request", slog.String("name", sq.name), slog.String("method", req.method))
			if !more {
				return
			}
			if localBuilder != nil {
				localBuilder.SendRequest(sq.log, req)
			}
			if !req.publicEndpoint {
				for _, peer := range peers {
					peer.SendRequest(sq.log, req)
				}
			}
		case newPeers, more := <-sq.updatePeers:
			if !more {
				return
			}
			for _, peer := range peers {
				peer.Close()
			}
			peers = nil
			for _, info := range newPeers {
				// don't send to yourself
				if info.OrderflowProxy.EcdsaPubkeyAddress == sq.signer.Address() {
					continue
				}
				client, err := RPCClientWithCertAndSigner(OrderflowProxyURLFromIP(info.IP), []byte(info.OrderflowProxy.TLSCert), sq.signer)
				if err != nil {
					sq.log.Error("Failed to create a peer client", slog.Any("error", err))
					shareQueueInternalErrors.Inc()
					continue
				}
				sq.log.Info("Created client for peer", slog.String("peer", info.Name), slog.String("name", sq.name))
				newPeer := newShareQueuePeer(info.Name, client)
				peers = append(peers, newPeer)
				go sq.proxyRequests(&newPeer)
			}
		}
	}
}

func (sq *ShareQueue) proxyRequests(peer *shareQueuePeer) {
	logger := sq.log.With(slog.String("peer", peer.name), slog.String("name", sq.name))
	for {
		req, more := <-peer.ch
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
			logger.Error("Unknown request type", slog.String("method", req.method))
			shareQueueInternalErrors.Inc()
			continue
		}
		start := time.Now()
		resp, err := peer.client.Call(ctx, method, data)
		timeShareQueuePeerRPCDuration(peer.name, int64(time.Since(start).Milliseconds()))
		if err != nil {
			logger.Warn("Error while proxying request", slog.Any("error", err))
			incShareQueuePeerRPCErrors(peer.name)
		}
		if resp != nil && resp.Error != nil {
			logger.Warn("Error returned form target while proxying", slog.Any("error", resp.Error))
			incShareQueuePeerRPCErrors(peer.name)
		}
		incShareQueueTotalRequests(peer.name)
		logger.Debug("Message proxied")
	}
}
