package llmobs

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
)

// Module provides the Recorder and registers its Prometheus collectors.
var Module = fx.Options(
	fx.Provide(
		New,
		provideCollectors,
	),
)

type collectorResult struct {
	fx.Out

	Classifications prometheus.Collector `group:"prometheus_collectors"`
	Duration        prometheus.Collector `group:"prometheus_collectors"`
}

func provideCollectors(r *Recorder) collectorResult {
	return collectorResult{
		Classifications: r.metrics.classifications,
		Duration:        r.metrics.duration,
	}
}
