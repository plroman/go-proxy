package metrics

import "github.com/VictoriaMetrics/metrics"

var (
	totalRequestReceived = metrics.NewCounter("requests_total")
)

func IncRequestsReceived() {
	totalRequestReceived.Inc()
}
