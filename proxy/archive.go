package proxy

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-utils/rpcclient"
	"github.com/flashbots/go-utils/rpctypes"
)

const NewOrderEventsMethod = "flashbots_newOrderEvents"

var (
	// ArchiveBatchSize is a maximum size of the batch to send to the archive
	ArchiveBatchSize = 100
	// ArchiveBatchSizeFlushTimeout is a timeout to force flush the batch to the archive
	ArchiveBatchSizeFlushTimeout = time.Second * 6

	errArchivePublicRequest = errors.New("public RPC request should not reach archive")

	ArchiveRequestTimeout = time.Second * 30
)

type ArchiveQueue struct {
	log               *slog.Logger
	queue             chan *ParsedRequest
	flushQueue        chan struct{}
	archiveClient     rpcclient.RPCClient
	blockNumberSource *BlockNumberSource
	workerCount       int
}

func (aq *ArchiveQueue) Run() {
	var workers []*archiveQueueWorker
	workerCount := 1
	if aq.workerCount > 0 {
		workerCount = aq.workerCount
	}
	workersQueue := make(chan *ParsedRequest)
	for w := 0; w < workerCount; w++ {
		worker := &archiveQueueWorker{
			log:           aq.log.With(slog.Int("worker", w)),
			archiveClient: aq.archiveClient,
			queue:         workersQueue,
			flushQueue:    make(chan struct{}),
		}
		go worker.runWorker()
		workers = append(workers, worker)
	}
	aq.log.Info("Started archival workers", slog.Int("workers", workerCount))
	defer func() {
		for _, worker := range workers {
			worker.close()
		}
		aq.log.Info("Stopped archival workers", slog.Int("workers", workerCount))
	}()

	var (
		flushTimer = time.After(ArchiveBatchSizeFlushTimeout)
		needFlush  = false
	)
	for {
		if needFlush {
			for _, worker := range workers {
				select {
				case worker.flushQueue <- struct{}{}:
				default:
				}
			}
			needFlush = false
			flushTimer = time.After(ArchiveBatchSizeFlushTimeout)
		}
		select {
		case _, more := <-aq.flushQueue:
			if !more {
				return
			}
			needFlush = true
		case <-flushTimer:
			needFlush = true
		case req, more := <-aq.queue:
			if !more {
				return
			}
			archiveEventsProcessedTotalCounter.Inc()
			processedReq, err := aq.updateParsedRequest(req)
			if err != nil {
				aq.log.Error("Failed to prepare request for archive", slog.Any("error", err))
				archiveEventsProcessedErrCounter.Inc()
				continue
			}
			if processedReq == nil {
				continue
			}
			select {
			case workersQueue <- processedReq:
			default:
				aq.log.Error("Archive workers are stalling")
			}
		}
	}
}

// updateParsedRequest will return updated request that can be used to send data to orderflow archive
// result can be nil without error meaning we don't need to archive that
func (aq *ArchiveQueue) updateParsedRequest(input *ParsedRequest) (*ParsedRequest, error) {
	if input.publicEndpoint {
		return nil, errArchivePublicRequest
	}
	if input.bidSubsidiseBlock != nil {
		return nil, nil
	}
	if input.ethSendRawTransaction != nil {
		var mevSendBundle rpctypes.MevSendBundleArgs
		block, err := aq.blockNumberSource.BlockNumber()
		if err != nil {
			return nil, err
		}
		mevSendBundle.Version = "v0.1"
		mevSendBundle.Inclusion.BlockNumber = hexutil.Uint64(block)
		mevSendBundle.Inclusion.MaxBlock = hexutil.Uint64(block + 5)
		mevSendBundle.Body = []rpctypes.MevBundleBody{{Tx: (*hexutil.Bytes)(input.ethSendRawTransaction), CanRevert: true}}
		signer := input.signer
		mevSendBundle.Metadata = &rpctypes.MevBundleMetadata{
			Signer: &signer,
		}

		input = &ParsedRequest{
			publicEndpoint: input.publicEndpoint,
			signer:         input.signer,
			method:         input.method,
			receivedAt:     input.receivedAt,
			mevSendBundle:  &mevSendBundle,
		}
	}
	return input, nil
}

type archiveQueueWorker struct {
	log           *slog.Logger
	archiveClient rpcclient.RPCClient
	queue         chan *ParsedRequest
	flushQueue    chan struct{}
}

func (aqw *archiveQueueWorker) close() {
	close(aqw.queue)
	close(aqw.flushQueue)
}

func (aqw *archiveQueueWorker) runWorker() {
	var pendingBatch []*ParsedRequest
	for {
		select {
		case req, more := <-aqw.queue:
			if !more {
				return
			}
			pendingBatch = append(pendingBatch, req)
		case _, more := <-aqw.flushQueue:
			if !more {
				return
			}
			aqw.flush(pendingBatch)
			pendingBatch = nil
		}
	}
}

func (aqw *archiveQueueWorker) flush(batch []*ParsedRequest) {
	args := FlashbotsNewOrderEventsArgs{}
	for _, request := range batch {
		event := ArchiveEvent{}
		metadata := ArchiveEventMetadata{
			ReceivedAt: request.receivedAt.UnixMilli(),
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
			aqw.log.Error("Incorrect request for orderflow archival", slog.String("method", request.method))
			archiveEventsProcessedErrCounter.Inc()
			continue
		}
		args.OrderEvents = append(args.OrderEvents, event)
	}
	if len(args.OrderEvents) == 0 {
		return
	}
	aqw.log.Info("Sending batch to the archive", slog.Int("size", len(args.OrderEvents)))
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), ArchiveRequestTimeout)
	defer cancel()
	res, err := aqw.archiveClient.Call(ctx, NewOrderEventsMethod, args)
	archiveEventsRPCDuration.Update(float64(time.Since(start).Milliseconds()))

	callFailed := false
	if err != nil {
		aqw.log.Error("Error while making RPC request to archive", slog.Any("error", err))
		archiveEventsRPCErrors.Inc()
		callFailed = true
	}
	if res != nil && res.Error != nil {
		aqw.log.Error("Archive returned error", slog.Any("error", res.Error))
		archiveEventsRPCErrors.Inc()
		callFailed = true
	}
	if !callFailed {
		archiveEventsRPCSentCounter.AddInt64(int64(len(args.OrderEvents)))
	}
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
	// ReceivedAt is a unix millisecond timestamp
	ReceivedAt int64 `json:"receivedAt"`
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
