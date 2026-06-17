package llmmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ClassificationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "llm_classify_duration_seconds",
			Help:    "Duration of LLM classification requests in seconds",
			Buckets: []float64{0.5, 1, 2, 3, 5, 8, 13, 21},
		},
		[]string{"provider", "status"},
	)

	ClassificationTokens = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llm_classify_tokens_total",
			Help: "Total tokens consumed by LLM classification",
		},
		[]string{"provider", "type"},
	)

	ClassificationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llm_classify_errors_total",
			Help: "Total LLM classification errors",
		},
		[]string{"provider", "error_type"},
	)

	ClassificationBatchSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "llm_classify_batch_size",
			Help:    "Number of torrents processed per batch",
			Buckets: []float64{1, 2, 5, 10, 20, 50},
		},
	)
)

func init() {
	prometheus.MustRegister(ClassificationDuration)
	prometheus.MustRegister(ClassificationTokens)
	prometheus.MustRegister(ClassificationErrors)
	prometheus.MustRegister(ClassificationBatchSize)
}
