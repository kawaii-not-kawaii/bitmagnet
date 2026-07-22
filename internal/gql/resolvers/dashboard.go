package resolvers

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
)

func (r *Resolver) dashboardQuery(ctx context.Context) (gen.DashboardQuery, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	total, err := r.Dao.Torrent.WithContext(ctx).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count torrents: %w", err)
	}

	today, err := r.Dao.Torrent.WithContext(ctx).Where(r.Dao.Torrent.CreatedAt.Gte(startOfDay)).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count today's torrents: %w", err)
	}

	lastHour, err := r.Dao.Torrent.WithContext(ctx).Where(r.Dao.Torrent.CreatedAt.Gte(now.Add(-time.Hour))).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count last hour: %w", err)
	}

	previousHour, err := r.Dao.Torrent.WithContext(ctx).Where(
		r.Dao.Torrent.CreatedAt.Gte(now.Add(-2*time.Hour)),
		r.Dao.Torrent.CreatedAt.Lt(now.Add(-time.Hour)),
	).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count previous hour: %w", err)
	}

	classified, err := r.Dao.TorrentContent.WithContext(ctx).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count classified torrents: %w", err)
	}

	processed, err := r.Dao.QueueJob.WithContext(ctx).Where(
		r.Dao.QueueJob.Status.Eq(string(model.QueueJobStatusProcessed)),
	).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count processed jobs: %w", err)
	}

	pending, err := r.Dao.QueueJob.WithContext(ctx).Where(
		r.Dao.QueueJob.Status.In(string(model.QueueJobStatusPending), string(model.QueueJobStatusRetry)),
	).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count pending jobs: %w", err)
	}

	failed, err := r.Dao.QueueJob.WithContext(ctx).Where(
		r.Dao.QueueJob.Status.Eq(string(model.QueueJobStatusFailed)),
	).Count()
	if err != nil {
		return gen.DashboardQuery{}, fmt.Errorf("dashboard: count failed jobs: %w", err)
	}

	classifiedPercent := 0.0
	if total > 0 {
		classifiedPercent = min(100, 100*float64(classified)/float64(total))
	}

	unknownBacklog := max(0, total-classified)

	return gen.DashboardQuery{
		Summary: gen.DashboardSummary{
			TotalTorrents:       int(total),
			TorrentsToday:       int(today),
			IndexedLastHour:     int(lastHour),
			IndexedPreviousHour: int(previousHour),
			ClassifiedPercent:   classifiedPercent,
			QueueProcessed:      int(processed),
			QueuePending:        int(pending),
			QueueFailed:         int(failed),
		},
		Llm: gen.DashboardLlm{
			State:   r.dashboardLlmState(),
			Metrics: r.dashboardLlmMetrics(int(unknownBacklog)),
		},
	}, nil
}

func (r *Resolver) dashboardLlmState() gen.DashboardLlmState {
	cfg := r.LlmRegistry.Config()
	name, providerCfg, _ := firstProviderConfig(cfg)

	return gen.DashboardLlmState{
		Enabled:         cfg.Enabled,
		Running:         len(r.LlmRegistry.All()) > 0,
		ProviderName:    name,
		BaseURL:         providerCfg.BaseURL,
		Model:           providerCfg.Model,
		APIKeySet:       providerCfg.APIKey != "",
		BatchSize:       cfg.BatchSize,
		MaxContext:      cfg.MaxContext,
		MaxTokens:       cfg.MaxTokens,
		IntervalSeconds: int(cfg.Interval.Seconds()),
		TimeoutSeconds:  int(cfg.Timeout.Seconds()),
	}
}

func (r *Resolver) dashboardLlmMetrics(unknownBacklog int) gen.DashboardLlmMetrics {
	snapshot := r.LlmStats.Snapshot(time.Now())
	distribution := make([]gen.DashboardLlmBenchmarkDistribution, 0, len(snapshot.Distribution))

	for contentType, count := range snapshot.Distribution {
		distribution = append(distribution, gen.DashboardLlmBenchmarkDistribution{
			ContentType: contentType,
			Count:       count,
		})
	}

	sort.Slice(distribution, func(i, j int) bool {
		return distribution[i].Count > distribution[j].Count
	})

	return gen.DashboardLlmMetrics{
		WindowSeconds:         snapshot.WindowSeconds,
		Matched:               snapshot.Matched,
		PromptTokens:          snapshot.PromptTokens,
		CompletionTokens:      snapshot.CompletionTokens,
		AverageLatencySeconds: snapshot.AverageLatency,
		P95LatencySeconds:     snapshot.P95Latency,
		ThroughputPerSecond:   snapshot.Throughput,
		ErrorRate:             snapshot.ErrorRate,
		UnknownBacklog:        unknownBacklog,
		Distribution:          distribution,
	}
}

func (r *Resolver) updateDashboardLlm(input gen.DashboardLlmConfigInput) (gen.DashboardLlmState, error) {
	name := strings.TrimSpace(input.ProviderName)
	baseURL := strings.TrimSpace(input.BaseURL)
	modelName := strings.TrimSpace(input.Model)

	if err := validateDashboardLlmInput(input, name, baseURL, modelName); err != nil {
		return gen.DashboardLlmState{}, err
	}

	current := r.LlmRegistry.Config()
	_, currentProvider, _ := firstProviderConfig(current)

	apiKey := currentProvider.APIKey
	if input.APIKey.IsSet() {
		apiKey = ""
		if value := input.APIKey.Value(); value != nil {
			apiKey = *value
		}
	}

	cfg := llm.RegistryConfig{
		Enabled: input.Enabled,
		Providers: map[string]llm.ProviderConfig{
			name: {
				BaseURL: baseURL,
				Model:   modelName,
				APIKey:  apiKey,
				Timeout: time.Duration(input.TimeoutSeconds) * time.Second,
			},
		},
		BatchSize:  input.BatchSize,
		MaxContext: input.MaxContext,
		MaxTokens:  input.MaxTokens,
		Interval:   time.Duration(input.IntervalSeconds) * time.Second,
		Timeout:    time.Duration(input.TimeoutSeconds) * time.Second,
	}
	if err := r.LlmRegistry.UpdateAndFlush(cfg); err != nil {
		return gen.DashboardLlmState{}, fmt.Errorf("dashboard: save LLM config: %w", err)
	}

	return r.dashboardLlmState(), nil
}

func validateDashboardLlmInput(input gen.DashboardLlmConfigInput, name, baseURL, modelName string) error {
	if name == "" {
		return fmt.Errorf("dashboard: provider name is required")
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return fmt.Errorf("dashboard: base URL must be an absolute HTTP or HTTPS URL")
	}

	if modelName == "" {
		return fmt.Errorf("dashboard: model is required")
	}

	if input.BatchSize < 1 || input.BatchSize > 100 {
		return fmt.Errorf("dashboard: batch size must be between 1 and 100")
	}

	if input.MaxContext < 1 || input.MaxTokens < 1 {
		return fmt.Errorf("dashboard: context and output token limits must be positive")
	}

	if input.IntervalSeconds < 1 || input.IntervalSeconds > 3600 {
		return fmt.Errorf("dashboard: batch interval must be between 1 and 3600 seconds")
	}

	if input.TimeoutSeconds < 1 || input.TimeoutSeconds > 600 {
		return fmt.Errorf("dashboard: timeout must be between 1 and 600 seconds")
	}

	return nil
}

func (r *Resolver) testDashboardLlmConnection(ctx context.Context) (gen.DashboardLlmConnectionResult, error) {
	_, provider, err := r.dashboardProvider()
	if err != nil {
		return gen.DashboardLlmConnectionResult{}, err
	}

	startedAt := time.Now()

	_, err = provider.Classify(ctx, llm.ClassifyInput{
		Name:         "The Matrix 1999 1080p BluRay",
		ContentTypes: strings.Join(model.ContentTypeNames(), ", "),
	})
	if err != nil {
		return gen.DashboardLlmConnectionResult{}, fmt.Errorf("dashboard: test LLM connection: %w", err)
	}

	return gen.DashboardLlmConnectionResult{
		Connected:      true,
		LatencySeconds: time.Since(startedAt).Seconds(),
	}, nil
}

func (r *Resolver) runDashboardLlmBenchmark(ctx context.Context, sampleSize int) (gen.DashboardLlmBenchmark, error) {
	if sampleSize < 1 || sampleSize > 100 {
		return gen.DashboardLlmBenchmark{}, fmt.Errorf(
			"dashboard: benchmark sample size must be between 1 and 100",
		)
	}

	_, provider, err := r.dashboardProvider()
	if err != nil {
		return gen.DashboardLlmBenchmark{}, err
	}

	torrents, err := r.Dao.Torrent.WithContext(ctx).Preload(r.Dao.Torrent.Files).Limit(sampleSize).Find()
	if err != nil {
		return gen.DashboardLlmBenchmark{}, fmt.Errorf("dashboard: load benchmark torrents: %w", err)
	}

	if len(torrents) == 0 {
		return gen.DashboardLlmBenchmark{}, fmt.Errorf("dashboard: no torrents available for benchmark")
	}

	type benchmarkSample struct {
		result   *llm.ClassifyResult
		duration float64
	}

	contentTypes := strings.Join(model.ContentTypeNames(), ", ")
	results := make(chan benchmarkSample, len(torrents))

	var wg sync.WaitGroup

	concurrency := min(max(1, r.LlmRegistry.Config().BatchSize), 10)
	semaphore := make(chan struct{}, concurrency)
	startedAt := time.Now()

	for _, torrent := range torrents {
		wg.Add(1)

		go func(torrent *model.Torrent) {
			defer wg.Done()
			semaphore <- struct{}{}

			defer func() { <-semaphore }()

			files := make([]string, 0, min(len(torrent.Files), 20))

			for i, file := range torrent.Files {
				if i == 20 {
					break
				}

				files = append(files, file.Path)
			}

			classifyStartedAt := time.Now()

			result, classifyErr := provider.Classify(ctx, llm.ClassifyInput{
				Name:         torrent.Name,
				Files:        files,
				ContentTypes: contentTypes,
			})
			if classifyErr != nil {
				results <- benchmarkSample{duration: time.Since(classifyStartedAt).Seconds()}
				return
			}
			results <- benchmarkSample{result: result, duration: time.Since(classifyStartedAt).Seconds()}
		}(torrent)
	}

	wg.Wait()
	close(results)

	distributionCounts := make(map[string]int)
	successes := 0

	latencySum := 0.0
	for sample := range results {
		latencySum += sample.duration

		if sample.result == nil {
			continue
		}

		successes++
		distributionCounts[sample.result.ContentType]++
	}

	duration := time.Since(startedAt).Seconds()

	distribution := make([]gen.DashboardLlmBenchmarkDistribution, 0, len(distributionCounts))
	for contentType, count := range distributionCounts {
		distribution = append(
			distribution,
			gen.DashboardLlmBenchmarkDistribution{ContentType: contentType, Count: count},
		)
	}

	sort.Slice(
		distribution,
		func(i, j int) bool { return distribution[i].Count > distribution[j].Count },
	)

	return gen.DashboardLlmBenchmark{
		SampleSize:            len(torrents),
		Successes:             successes,
		Failures:              len(torrents) - successes,
		AverageLatencySeconds: latencySum / float64(len(torrents)),
		ThroughputPerSecond:   float64(len(torrents)) / max(duration, 0.001),
		Distribution:          distribution,
	}, nil
}

func (r *Resolver) dashboardProvider() (string, llm.Provider, error) {
	providers := r.LlmRegistry.All()
	names := make([]string, 0, len(providers))

	for name := range providers {
		names = append(names, name)
	}

	if len(names) == 0 {
		return "", nil, fmt.Errorf("dashboard: LLM engine is disabled or not configured")
	}

	sort.Strings(names)

	return names[0], providers[names[0]], nil
}

func firstProviderConfig(cfg llm.RegistryConfig) (string, llm.ProviderConfig, bool) {
	names := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		names = append(names, name)
	}

	if len(names) == 0 {
		return "", llm.ProviderConfig{}, false
	}

	sort.Strings(names)

	return names[0], cfg.Providers[names[0]], true
}
