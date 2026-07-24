package llmobs

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

const metricsNamespace = "bitmagnet"

type recorderMetrics struct {
	classifications *prometheus.CounterVec
	duration        *prometheus.HistogramVec
}

func newRecorderMetrics() recorderMetrics {
	return recorderMetrics{
		classifications: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "llm",
			Name:      "classifications_total",
			Help:      "Total LLM classification attempts by provider, outcome, and error category.",
		}, []string{"provider", "outcome", "category"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "llm",
			Name:      "classification_duration_seconds",
			Help:      "Duration of LLM classification attempts in seconds.",
		}, []string{"provider"}),
	}
}

func (m recorderMetrics) record(e Event) {
	if m.classifications != nil {
		m.classifications.WithLabelValues(e.Provider, strings.ToLower(string(e.Outcome)), string(e.Category)).
			Inc()
	}

	if m.duration != nil {
		m.duration.WithLabelValues(e.Provider).Observe(e.Duration.Seconds())
	}
}

// Collectors returns the Recorder's Prometheus collectors for registration.
func (r *Recorder) Collectors() []prometheus.Collector {
	if r == nil {
		return nil
	}

	collectors := make([]prometheus.Collector, 0, 2)
	if r.metrics.classifications != nil {
		collectors = append(collectors, r.metrics.classifications)
	}

	if r.metrics.duration != nil {
		collectors = append(collectors, r.metrics.duration)
	}

	return collectors
}
