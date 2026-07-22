package llm

import (
	"math"
	"sync"
	"time"
)

const statsWindowMinutes = 24 * 60

var latencyBounds = [...]float64{0.5, 1, 2, 3, 5, 8, 13, 21, 30}

// Observation describes one provider request.
type Observation struct {
	Provider         string
	At               time.Time
	Duration         time.Duration
	PromptTokens     int
	CompletionTokens int
	Classifications  int
	ContentTypes     []string
	Failed           bool
}

// StatsSnapshot contains aggregate provider activity from the last 24 hours.
type StatsSnapshot struct {
	WindowSeconds    int
	Matched          int
	PromptTokens     int
	CompletionTokens int
	AverageLatency   float64
	P95Latency       float64
	Throughput       float64
	ErrorRate        float64
	Distribution     map[string]int
}

type statsBucket struct {
	minute           int64
	requests         int
	failures         int
	matched          int
	promptTokens     int
	completionTokens int
	durationSeconds  float64
	maxDuration      float64
	latencies        [len(latencyBounds) + 1]int
	distribution     map[string]int
}

// Stats keeps fixed-size, minute-granularity request statistics for the last 24 hours.
type Stats struct {
	mu        sync.Mutex
	startedAt time.Time
	buckets   [statsWindowMinutes]statsBucket
}

func NewStats() *Stats {
	return &Stats{startedAt: time.Now()}
}

func (s *Stats) Record(observation Observation) {
	if observation.At.IsZero() {
		observation.At = time.Now()
	}

	minute := observation.At.Unix() / 60
	index := int(minute % statsWindowMinutes)

	s.mu.Lock()
	defer s.mu.Unlock()

	bucket := &s.buckets[index]
	if bucket.minute != minute {
		*bucket = statsBucket{minute: minute}
	}

	seconds := observation.Duration.Seconds()
	bucket.requests++
	bucket.matched += observation.Classifications
	bucket.promptTokens += observation.PromptTokens
	bucket.completionTokens += observation.CompletionTokens
	bucket.durationSeconds += seconds
	bucket.maxDuration = max(bucket.maxDuration, seconds)
	if observation.Failed {
		bucket.failures++
	}
	for _, contentType := range observation.ContentTypes {
		if bucket.distribution == nil {
			bucket.distribution = make(map[string]int)
		}
		bucket.distribution[contentType]++
	}

	latencyIndex := len(latencyBounds)
	for i, bound := range latencyBounds {
		if seconds <= bound {
			latencyIndex = i
			break
		}
	}
	bucket.latencies[latencyIndex]++

	status := "success"
	if observation.Failed {
		status = "error"
		ClassificationErrors.WithLabelValues(observation.Provider, "request").Inc()
	}
	ClassificationDuration.WithLabelValues(observation.Provider, status).Observe(seconds)
	ClassificationTokens.WithLabelValues(observation.Provider, "prompt").Add(float64(observation.PromptTokens))
	ClassificationTokens.WithLabelValues(observation.Provider, "completion").Add(float64(observation.CompletionTokens))
	ClassificationBatchSize.Observe(float64(observation.Classifications))
}

func (s *Stats) Snapshot(now time.Time) StatsSnapshot {
	if now.IsZero() {
		now = time.Now()
	}

	cutoff := now.Unix()/60 - statsWindowMinutes + 1

	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		result       StatsSnapshot
		latencies    [len(latencyBounds) + 1]int
		durationSum  float64
		maxDuration  float64
		requestCount int
		failureCount int
	)
	result.Distribution = make(map[string]int)

	for i := range s.buckets {
		bucket := &s.buckets[i]
		if bucket.minute < cutoff {
			continue
		}
		result.Matched += bucket.matched
		result.PromptTokens += bucket.promptTokens
		result.CompletionTokens += bucket.completionTokens
		durationSum += bucket.durationSeconds
		maxDuration = max(maxDuration, bucket.maxDuration)
		requestCount += bucket.requests
		failureCount += bucket.failures
		for contentType, count := range bucket.distribution {
			result.Distribution[contentType] += count
		}
		for j, count := range bucket.latencies {
			latencies[j] += count
		}
	}

	window := min(now.Sub(s.startedAt), 24*time.Hour)
	result.WindowSeconds = max(1, int(window.Seconds()))
	if requestCount == 0 {
		return result
	}

	result.AverageLatency = durationSum / float64(requestCount)
	result.Throughput = float64(result.Matched) / float64(result.WindowSeconds)
	result.ErrorRate = 100 * float64(failureCount) / float64(requestCount)

	target := int(math.Ceil(float64(requestCount) * 0.95))
	seen := 0
	for i, count := range latencies {
		seen += count
		if seen < target {
			continue
		}
		if i < len(latencyBounds) {
			result.P95Latency = latencyBounds[i]
		} else {
			result.P95Latency = maxDuration
		}
		break
	}

	return result
}
