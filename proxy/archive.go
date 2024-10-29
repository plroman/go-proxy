package proxy

// TODO: what if archive stalls? make sure that spikes are processed properly

import (
	"context"
	"log/slog"
	"time"

	"github.com/flashbots/go-utils/rpcclient"
	"github.com/flashbots/go-utils/rpctypes"
)

const NewOrderEventsMethod = "flashbots_newOrderEvents"

var (
	// ArchiveBatchSize is a maximum size of the batch to send to the archive
	ArchiveBatchSize = 100
	// ArchiveBatchSizeFlushTimeout is a timeout to force flush the batch to the archive
	ArchiveBatchSizeFlushTimeout = time.Second * 6
)

type ArchiveQueue struct {
	log           *slog.Logger
	queue         chan *ParsedRequest
	flushQueue    chan struct{}
	archiveClient rpcclient.RPCClient
}

func (aq *ArchiveQueue) Run() {
	var (
		flushTimer   = time.After(ArchiveBatchSizeFlushTimeout)
		pendingBatch []*ParsedRequest
		forceFlush   = false
	)
	for {
		needFlush := forceFlush || len(pendingBatch) >= ArchiveBatchSize
		if len(pendingBatch) == 0 {
			needFlush = false
		}

		if needFlush {
			aq.flush(pendingBatch)
		}

		forceFlush = false
		select {
		case req, more := <-aq.queue:
			if !more {
				return
			}
			processedReq, err := aq.updateParsedRequest(req)
			if err != nil {
				aq.log.Error("Failed to prepare request for archive", slog.Any("error", err))
				continue
			}
			if processedReq == nil {
				continue
			}
			pendingBatch = append(pendingBatch, req)
		case _, more := <-aq.flushQueue:
			if !more {
				return
			}
			forceFlush = true
		case <-flushTimer:
			forceFlush = true
			flushTimer = time.After(ArchiveBatchSizeFlushTimeout)
		}
	}
}

// updateParsedRequest will return updated request that can be used to send data to orderflow archive
// result can be nil without error meaning we don't need to archive that
func (aq *ArchiveQueue) updateParsedRequest(input *ParsedRequest) (*ParsedRequest, error) {
	if input.publicEndpoint {
		return nil, nil
	}
	if input.bidSubsidiseBlock != nil {
		return nil, nil
	}
	// TODO: convert raw transaction to mev send bundle
	return input, nil
}

type FlashbotsNewOrderEventsArgs struct {
	OrderEvents []ArchiveEvent `json:"orderEvents"`
}

type ArchiveEvent struct {
	EthSendBundle   *ArchiveEventEthSendBundle   `json:"eth_sendBundle,omitempty"`
	MevSendBundle   *ArchiveEventMevSendBundle   `json:"mev_sendBundle,omitempty"`
	EthCancelBundle *ArchiveEventEthCancelBundle `json:"eth_cancelBundle,omitempty"`
}

type ArchiveEventMetadata struct {
	// Timestamp is a unix millisecond receivedAt
	Timestamp int64 `json:"receivedAt"`
}

type ArchiveEventEthSendBundle struct {
	Params   *rpctypes.EthSendBundleArgs `json:"params"`
	Metadata *ArchiveEventMetadata       `json:"metadata"`
}

type ArchiveEventMevSendBundle struct {
	Params   *rpctypes.MevSendBundleArgs `json:"params"`
	Metadata *ArchiveEventMetadata       `json:"metadata"`
}

type ArchiveEventEthCancelBundle struct {
	Params   *rpctypes.EthCancelBundleArgs `json:"params"`
	Metadata *ArchiveEventMetadata         `json:"metadata"`
}

func (aq *ArchiveQueue) flush(batch []*ParsedRequest) {
	// @metric
	aq.log.Info("Sending batch to the archive", slog.Int("size", len(batch)))
	args := FlashbotsNewOrderEventsArgs{}
	for _, request := range batch {
		event := ArchiveEvent{}
		metadata := ArchiveEventMetadata{
			Timestamp: request.receivedAt.UnixMilli(),
		}
		if request.ethSendBundle != nil {
			event.EthSendBundle = &ArchiveEventEthSendBundle{
				Params:   request.ethSendBundle,
				Metadata: &metadata,
			}
		} else if request.mevSendBundle != nil {
			event.MevSendBundle = &ArchiveEventMevSendBundle{
				Params:   request.mevSendBundle,
				Metadata: &metadata,
			}
		} else if request.ethCancelBundle != nil {
			event.EthCancelBundle = &ArchiveEventEthCancelBundle{
				Params:   request.ethCancelBundle,
				Metadata: &metadata,
			}
		} else {
			aq.log.Error("Incorrect request for orderflow archival", slog.String("method", request.method))
			continue
		}
		args.OrderEvents = append(args.OrderEvents, event)
	}
	if len(args.OrderEvents) == 0 {
		return
	}
	res, err := aq.archiveClient.Call(context.Background(), NewOrderEventsMethod, args)
	if err != nil {
		aq.log.Error("Error while making RPC request to archive", slog.Any("error", err))
	}
	if res.Error != nil {
		aq.log.Error("Archive returned error", slog.Any("error", res.Error))
	}
}
