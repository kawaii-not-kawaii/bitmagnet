package tmdb

import "time"

type Config struct {
	Enabled        bool
	BaseURL        string
	APIKey         string
	RateLimit      time.Duration
	RateLimitBurst int
	// MaxConcurrentRequests caps TMDB requests in flight (each slot is held
	// through resty's retries). With ~0.3-0.7 s round trips the old fixed cap
	// of 2 limited throughput to a few requests/s regardless of RateLimit.
	MaxConcurrentRequests int
}

func NewDefaultConfig() Config {
	return Config{
		Enabled:        true,
		BaseURL:        "https://api.themoviedb.org/3",
		APIKey:         defaultTmdbAPIKey,
		RateLimit:      defaultRateLimit,
		RateLimitBurst: defaultRateLimitBurst,

		MaxConcurrentRequests: defaultMaxConcurrentRequests,
	}
}

// maxConcurrentRequests returns the configured in-flight cap, falling back to
// the default for unset or invalid values.
func maxConcurrentRequests(config Config) int64 {
	if config.MaxConcurrentRequests <= 0 {
		return defaultMaxConcurrentRequests
	}

	return int64(config.MaxConcurrentRequests)
}

const (
	defaultTmdbAPIKey     = "9c6689fa83ae6814fbfb200d70bba3a8"
	defaultRateLimit      = time.Second / 20
	defaultRateLimitBurst = 5

	defaultMaxConcurrentRequests = 2
)
