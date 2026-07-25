package openai

import (
	"testing"
	"time"
)

func TestRecommend(t *testing.T) {
	t.Parallel()

	zero := 0
	testCases := []struct {
		name     string
		capacity Capacity
		current  CurrentConfig
		latency  time.Duration
		want     RecommendedConfig
	}{
		{
			name:     "defaults",
			capacity: Capacity{Source: "unknown"},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      256,
				MaxContext:     16000,
				TimeoutSeconds: 30,
				Concurrency:    10,
			},
		},
		{
			name:     "detected slots set concurrency",
			capacity: Capacity{Source: "slots", Slots: positiveInt(16)},
			current:  CurrentConfig{MaxTokens: 384, MaxContext: 4000, Concurrency: 7},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      384,
				MaxContext:     4000,
				TimeoutSeconds: 30,
				Concurrency:    16,
			},
		},
		{
			name:     "models metadata uses hosted concurrency",
			capacity: Capacity{Source: "models"},
			current:  CurrentConfig{MaxTokens: 384, MaxContext: 4000, Concurrency: 7},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      384,
				MaxContext:     4000,
				TimeoutSeconds: 30,
				Concurrency:    16,
			},
		},
		{
			name:     "models dev metadata uses hosted concurrency",
			capacity: Capacity{Source: "models.dev"},
			current:  CurrentConfig{MaxTokens: 384, MaxContext: 4000, Concurrency: 7},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      384,
				MaxContext:     4000,
				TimeoutSeconds: 30,
				Concurrency:    16,
			},
		},
		{
			name:     "props retains current concurrency",
			capacity: Capacity{Source: "props"},
			current:  CurrentConfig{MaxTokens: 384, MaxContext: 4000, Concurrency: 7},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      384,
				MaxContext:     4000,
				TimeoutSeconds: 30,
				Concurrency:    7,
			},
		},
		{
			name:     "smaller completion limit caps output",
			capacity: Capacity{MaxCompletionTokens: positiveInt(128)},
			current:  CurrentConfig{MaxTokens: 512, MaxContext: 4000, Concurrency: 5},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      128,
				MaxContext:     4000,
				TimeoutSeconds: 30,
				Concurrency:    5,
			},
		},
		{
			name:     "larger completion limit does not inflate output",
			capacity: Capacity{MaxCompletionTokens: positiveInt(512)},
			current:  CurrentConfig{MaxTokens: 128, MaxContext: 4000, Concurrency: 5},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      128,
				MaxContext:     4000,
				TimeoutSeconds: 30,
				Concurrency:    5,
			},
		},
		{
			name:     "small context window keeps both budgets positive",
			capacity: Capacity{ContextPerRequest: positiveInt(1000)},
			current:  CurrentConfig{MaxTokens: 1200, MaxContext: 2000, Concurrency: 5},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      999,
				MaxContext:     1,
				TimeoutSeconds: 30,
				Concurrency:    5,
			},
		},
		{
			name:     "large context window does not inflate budgets",
			capacity: Capacity{ContextPerRequest: positiveInt(32000)},
			current:  CurrentConfig{MaxTokens: 512, MaxContext: 4000, Concurrency: 5},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      512,
				MaxContext:     4000,
				TimeoutSeconds: 30,
				Concurrency:    5,
			},
		},
		{
			name: "zero context and completion metadata are ignored",
			capacity: Capacity{
				ContextPerRequest:   &zero,
				MaxCompletionTokens: &zero,
			},
			current: CurrentConfig{MaxTokens: 17, MaxContext: 23, Concurrency: 5},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      17,
				MaxContext:     23,
				TimeoutSeconds: 30,
				Concurrency:    5,
			},
		},
		{
			name:     "one token context metadata is ignored",
			capacity: Capacity{ContextPerRequest: positiveInt(1)},
			current:  CurrentConfig{MaxTokens: 17, MaxContext: 23, Concurrency: 5},
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      17,
				MaxContext:     23,
				TimeoutSeconds: 30,
				Concurrency:    5,
			},
		},
		{
			name:    "latency below timeout floor",
			current: CurrentConfig{MaxTokens: 17, MaxContext: 23, Timeout: time.Hour, Concurrency: 5},
			latency: 7 * time.Second,
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      17,
				MaxContext:     23,
				TimeoutSeconds: 30,
				Concurrency:    5,
			},
		},
		{
			name:    "negative latency uses timeout floor",
			current: CurrentConfig{MaxTokens: 17, MaxContext: 23, Concurrency: 5},
			latency: -time.Second,
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      17,
				MaxContext:     23,
				TimeoutSeconds: 30,
				Concurrency:    5,
			},
		},
		{
			name:    "fractional latency rounds timeout up",
			current: CurrentConfig{MaxTokens: 17, MaxContext: 23, Concurrency: 5},
			latency: 10*time.Second + 125*time.Millisecond,
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      17,
				MaxContext:     23,
				TimeoutSeconds: 41,
				Concurrency:    5,
			},
		},
		{
			name:    "latency above timeout ceiling",
			current: CurrentConfig{MaxTokens: 17, MaxContext: 23, Concurrency: 5},
			latency: 76 * time.Second,
			want: RecommendedConfig{
				BatchSize:      1,
				MaxTokens:      17,
				MaxContext:     23,
				TimeoutSeconds: 300,
				Concurrency:    5,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Recommend(testCase.capacity, testCase.current, testCase.latency)
			if got != testCase.want {
				t.Fatalf("Recommend() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
