package proxy

import (
	"fmt"

	"github.com/VictoriaMetrics/metrics"
)

var (
	archiveEventsProcessedTotalCounter = metrics.NewCounter("orderflow_proxy_archive_events_processed_total")
	archiveEventsProcessedErrCounter   = metrics.NewCounter("orderflow_proxy_archive_events_processed_err")

	archiveEventsRPCErrors      = metrics.NewCounter("orderflow_proxy_events_rpc_errors")
	archiveEventsRPCSentCounter = metrics.NewCounter("orderflow_proxy_events_rpc_sent")
	archiveEventsRPCDuration    = metrics.NewSummary("orderflow_proxy_events_rpc_duration_milliseconds")

	confighubErrorsCounter = metrics.NewCounter("orderflow_proxy_confighub_errors")

	shareQueueInternalErrors = metrics.NewCounter("orderflow_proxy_share_queue_internal_errors")
	apiLocalRateLimits       = metrics.NewCounter("orderflow_proxy_api_local_rate_limits")
)

const (
	apiIncomingRequestsByPeer  = `orderflow_proxy_api_incoming_requests_by_peer{peer="%s"}`
	apiDuplicateRequestsByPeer = `orderflow_proxy_api_duplicate_requests_by_peer{peer="%s"}`

	shareQueuePeerTotalRequestsLabel  = `orderflow_proxy_share_queue_peer_total_requests{peer="%s"}`
	shareQueuePeerStallingErrorsLabel = `orderflow_proxy_share_queue_peer_stalling_errors{peer="%s"}`
	shareQueuePeerRPCErrorsLabel      = `orderflow_proxy_share_queue_peer_rpc_errors{peer="%s"}`
	shareQueuePeerRPCDurationLabel    = `orderflow_proxy_share_queue_peer_rpc_duration_milliseconds{peer="%s"}`
)

func incAPIIncomingRequestsByPeer(peer string) {
	l := fmt.Sprintf(apiIncomingRequestsByPeer, peer)
	metrics.GetOrCreateCounter(l).Inc()
}

func incAPIDuplicateRequestsByPeer(peer string) {
	l := fmt.Sprintf(apiDuplicateRequestsByPeer, peer)
	metrics.GetOrCreateCounter(l).Inc()
}

func incAPILocalRateLimits() {
	apiLocalRateLimits.Inc()
}

func incShareQueueTotalRequests(peer string) {
	l := fmt.Sprintf(shareQueuePeerTotalRequestsLabel, peer)
	metrics.GetOrCreateCounter(l).Inc()
}

func incShareQueuePeerStallingErrors(peer string) {
	l := fmt.Sprintf(shareQueuePeerStallingErrorsLabel, peer)
	metrics.GetOrCreateCounter(l).Inc()
}

func incShareQueuePeerRPCErrors(peer string) {
	l := fmt.Sprintf(shareQueuePeerRPCErrorsLabel, peer)
	metrics.GetOrCreateCounter(l).Inc()
}

func timeShareQueuePeerRPCDuration(peer string, duration int64) {
	l := fmt.Sprintf(shareQueuePeerRPCDurationLabel, peer)
	metrics.GetOrCreateSummary(l).Update(float64(duration))
}
