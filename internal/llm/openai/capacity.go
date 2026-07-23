package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	capacityProbeTimeout = 3 * time.Second
	modelsDevRegistryURL = "https://models.dev/api.json"
)

// Capacity is the best metadata reported by an OpenAI-compatible endpoint.
type Capacity struct {
	Source              string
	ContextPerRequest   *int
	MaxCompletionTokens *int
	Slots               *int
	Fits                *bool
	Message             string
}

type capacityBudgets struct {
	maxContext int
	maxTokens  int
}

type modelsDevLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type propsGenerationSettings struct {
	Context int `json:"n_ctx"`
}

type propsResponse struct {
	DefaultGenerationSettings propsGenerationSettings `json:"default_generation_settings"`
}

type modelsTopProvider struct {
	MaxCompletionTokens int `json:"max_completion_tokens"`
}

type modelsResponseModel struct {
	ID            string            `json:"id"`
	ContextLength int               `json:"context_length"`
	TopProvider   modelsTopProvider `json:"top_provider"`
}

type modelsResponse struct {
	Data []modelsResponseModel `json:"data"`
}

type modelsDevCache struct {
	mutex    sync.Mutex
	registry modelsDevRegistry
}

type modelsDevRegistry map[string]struct {
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID    string         `json:"id"`
	Limit modelsDevLimit `json:"limit"`
}

var sharedModelsDevCache modelsDevCache

// ProbeCapacity discovers model capacity without spending completion tokens.
func ProbeCapacity(
	ctx context.Context,
	httpClient *http.Client,
	baseURL string,
	apiKey string,
	model string,
	maxContext int,
	maxTokens int,
) Capacity {
	return probeCapacity(
		ctx,
		httpClient,
		baseURL,
		apiKey,
		model,
		capacityBudgets{maxContext: maxContext, maxTokens: maxTokens},
		modelsDevRegistryURL,
		&sharedModelsDevCache,
	)
}

func probeCapacity(
	ctx context.Context,
	httpClient *http.Client,
	baseURL string,
	apiKey string,
	model string,
	budgets capacityBudgets,
	registryURL string,
	registryCache *modelsDevCache,
) Capacity {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	baseURL = strings.TrimRight(baseURL, "/")
	if capacity, ok := probeSlots(ctx, httpClient, baseURL+"/v1/slots", apiKey); ok {
		return finishCapacity(capacity, budgets.maxContext, budgets.maxTokens)
	}

	if capacity, ok := probeProps(ctx, httpClient, baseURL+"/props", apiKey); ok {
		return finishCapacity(capacity, budgets.maxContext, budgets.maxTokens)
	}

	if capacity, ok := probeModels(ctx, httpClient, baseURL+"/v1/models", apiKey, model); ok {
		return finishCapacity(capacity, budgets.maxContext, budgets.maxTokens)
	}

	registry, err := registryCache.load(ctx, httpClient, registryURL)
	if err == nil {
		if modelMetadata, ok := findModelsDevModel(registry, model); ok {
			capacity := Capacity{Source: "models.dev"}
			capacity.ContextPerRequest = positiveInt(modelMetadata.Limit.Context)
			capacity.MaxCompletionTokens = positiveInt(modelMetadata.Limit.Output)

			if capacity.ContextPerRequest != nil || capacity.MaxCompletionTokens != nil {
				return finishCapacity(capacity, budgets.maxContext, budgets.maxTokens)
			}
		}
	}

	return Capacity{
		Source:  "unknown",
		Message: "capacity unknown — endpoint reports no metadata",
	}
}

func probeSlots(ctx context.Context, client *http.Client, url string, apiKey string) (Capacity, bool) {
	var slots []struct {
		Context int `json:"n_ctx"`
	}

	if getJSON(ctx, client, url, apiKey, &slots) != nil || len(slots) == 0 || slots[0].Context <= 0 {
		return Capacity{}, false
	}

	return Capacity{
		Source:            "slots",
		ContextPerRequest: positiveInt(slots[0].Context),
		Slots:             positiveInt(len(slots)),
	}, true
}

func probeProps(ctx context.Context, client *http.Client, url string, apiKey string) (Capacity, bool) {
	var props propsResponse

	if getJSON(ctx, client, url, apiKey, &props) != nil || props.DefaultGenerationSettings.Context <= 0 {
		return Capacity{}, false
	}

	return Capacity{
		Source:            "props",
		ContextPerRequest: positiveInt(props.DefaultGenerationSettings.Context),
	}, true
}

func probeModels(
	ctx context.Context,
	client *http.Client,
	url string,
	apiKey string,
	model string,
) (Capacity, bool) {
	var response modelsResponse

	if getJSON(ctx, client, url, apiKey, &response) != nil {
		return Capacity{}, false
	}

	for _, candidate := range response.Data {
		if candidate.ID != model {
			continue
		}

		capacity := Capacity{
			Source:              "models",
			ContextPerRequest:   positiveInt(candidate.ContextLength),
			MaxCompletionTokens: positiveInt(candidate.TopProvider.MaxCompletionTokens),
		}

		return capacity, capacity.ContextPerRequest != nil || capacity.MaxCompletionTokens != nil
	}

	return Capacity{}, false
}

func (cache *modelsDevCache) load(
	ctx context.Context,
	client *http.Client,
	url string,
) (modelsDevRegistry, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	if cache.registry != nil {
		return cache.registry, nil
	}

	var registry modelsDevRegistry
	if err := getJSON(ctx, client, url, "", &registry); err != nil {
		return nil, err
	}

	cache.registry = registry

	return registry, nil
}

func findModelsDevModel(registry modelsDevRegistry, model string) (modelsDevModel, bool) {
	providerNames := make([]string, 0, len(registry))
	for providerName := range registry {
		providerNames = append(providerNames, providerName)
	}

	sort.Strings(providerNames)

	for _, suffixMatch := range []bool{false, true} {
		for _, providerName := range providerNames {
			provider := registry[providerName]
			modelNames := make([]string, 0, len(provider.Models))

			for modelName := range provider.Models {
				modelNames = append(modelNames, modelName)
			}

			sort.Strings(modelNames)

			for _, modelName := range modelNames {
				metadata := provider.Models[modelName]

				candidate := metadata.ID
				if candidate == "" {
					candidate = modelName
				}

				if model == candidate || suffixMatch && strings.HasSuffix(model, "/"+candidate) {
					return metadata, true
				}
			}
		}
	}

	return modelsDevModel{}, false
}

func finishCapacity(capacity Capacity, maxContext int, maxTokens int) Capacity {
	if capacity.ContextPerRequest == nil {
		if capacity.MaxCompletionTokens != nil {
			capacity.Message = fmt.Sprintf(
				"max completion %d · capacity is concurrent-call bound — concurrency is your quota/cost throttle",
				*capacity.MaxCompletionTokens,
			)
		}

		return capacity
	}

	configured := maxContext + effectiveMaxTokens(maxTokens)
	fits := configured <= *capacity.ContextPerRequest
	capacity.Fits = &fits

	if capacity.Slots != nil {
		if fits {
			capacity.Message = fmt.Sprintf(
				"%d slots × %d ctx · config fits",
				*capacity.Slots,
				*capacity.ContextPerRequest,
			)
		} else {
			capacity.Message = fmt.Sprintf(
				"%d slots × %d ctx · max_context+max_tokens (%d) exceeds per-slot window",
				*capacity.Slots,
				*capacity.ContextPerRequest,
				configured,
			)
		}

		return capacity
	}

	capacity.Message = fmt.Sprintf(
		"context %d · capacity is concurrent-call bound — concurrency is your quota/cost throttle",
		*capacity.ContextPerRequest,
	)

	return capacity
}

func positiveInt(value int) *int {
	if value <= 0 {
		return nil
	}

	return &value
}

func getJSON(ctx context.Context, client *http.Client, url string, apiKey string, target any) error {
	requestCtx, cancel := context.WithTimeout(ctx, capacityProbeTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("capacity probe: HTTP %d", response.StatusCode)
	}

	return json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(target)
}
