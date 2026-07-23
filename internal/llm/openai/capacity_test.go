package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestProbeCapacity(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                string
		responses           map[string]probeResponse
		registry            string
		model               string
		maxContext          int
		maxTokens           int
		wantSource          string
		wantContext         int
		wantCompletion      int
		wantSlots           int
		wantFits            *bool
		wantMessage         string
		checkRegistryCached bool
	}{
		{
			name: "slots fit",
			responses: map[string]probeResponse{
				"/v1/slots": {body: `[{"n_ctx":8192},{"n_ctx":8192}]`},
			},
			model:       "local",
			maxContext:  7000,
			maxTokens:   256,
			wantSource:  "slots",
			wantContext: 8192,
			wantSlots:   2,
			wantFits:    boolPointer(true),
			wantMessage: "2 slots × 8192 ctx · config fits",
		},
		{
			name: "slots do not fit",
			responses: map[string]probeResponse{
				"/v1/slots": {body: `[{"n_ctx":8192},{"n_ctx":8192}]`},
			},
			model:       "local",
			maxContext:  8000,
			maxTokens:   512,
			wantSource:  "slots",
			wantContext: 8192,
			wantSlots:   2,
			wantFits:    boolPointer(false),
			wantMessage: "2 slots × 8192 ctx · max_context+max_tokens (8512) exceeds per-slot window",
		},
		{
			name: "props",
			responses: map[string]probeResponse{
				"/props": {body: `{"default_generation_settings":{"n_ctx":4096}}`},
			},
			model:       "local",
			maxContext:  4000,
			wantSource:  "props",
			wantContext: 4096,
			wantFits:    boolPointer(false),
			wantMessage: "context 4096 · capacity is concurrent-call bound — concurrency is your quota/cost throttle",
		},
		{
			name: "models after malformed and error responses",
			responses: map[string]probeResponse{
				"/v1/slots": {body: `not-json`},
				"/props":    {status: http.StatusInternalServerError},
				"/v1/models": {
					body: `{"data":[{"id":"hosted","context_length":128000,"top_provider":{"max_completion_tokens":4096}}]}`,
				},
			},
			model:          "hosted",
			maxContext:     120000,
			maxTokens:      512,
			wantSource:     "models",
			wantContext:    128000,
			wantCompletion: 4096,
			wantFits:       boolPointer(true),
			wantMessage:    "context 128000 · capacity is concurrent-call bound — concurrency is your quota/cost throttle",
		},
		{
			name:                "models.dev suffix match and cache",
			registry:            `{"openrouter":{"models":{"vendor/model":{"id":"vendor/model","limit":{"context":32000,"output":2048}}}}}`,
			model:               "gateway/vendor/model",
			maxContext:          30000,
			wantSource:          "models.dev",
			wantContext:         32000,
			wantCompletion:      2048,
			wantFits:            boolPointer(true),
			wantMessage:         "context 32000 · capacity is concurrent-call bound — concurrency is your quota/cost throttle",
			checkRegistryCached: true,
		},
		{
			name:        "unknown",
			registry:    `{}`,
			model:       "missing",
			maxContext:  8000,
			maxTokens:   256,
			wantSource:  "unknown",
			wantMessage: "capacity unknown — endpoint reports no metadata",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider := httptest.NewServer(
				http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.Header.Get("Authorization") != "Bearer secret" {
						t.Errorf(
							"provider authorization = %q",
							request.Header.Get("Authorization"),
						)
					}

					response, ok := testCase.responses[request.URL.Path]
					if !ok {
						http.NotFound(writer, request)

						return
					}

					if response.status != 0 {
						writer.WriteHeader(response.status)
					}

					_, _ = writer.Write([]byte(response.body))
				}),
			)
			defer provider.Close()

			var registryRequests atomic.Int32

			registry := httptest.NewServer(
				http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					registryRequests.Add(1)

					if authorization := request.Header.Get("Authorization"); authorization != "" {
						t.Errorf("models.dev authorization = %q", authorization)
					}

					_, _ = writer.Write([]byte(testCase.registry))
				}),
			)
			defer registry.Close()

			cache := &modelsDevCache{}
			capacity := probeCapacity(
				context.Background(),
				provider.Client(),
				provider.URL,
				"secret",
				testCase.model,
				testCase.maxContext,
				testCase.maxTokens,
				registry.URL,
				cache,
			)

			assertCapacity(t, capacity, testCase)

			if testCase.checkRegistryCached {
				second := probeCapacity(
					context.Background(),
					provider.Client(),
					provider.URL,
					"secret",
					testCase.model,
					testCase.maxContext,
					testCase.maxTokens,
					registry.URL,
					cache,
				)
				assertCapacity(t, second, testCase)

				if requests := registryRequests.Load(); requests != 1 {
					t.Fatalf("registry requests = %d, want 1", requests)
				}
			}
		})
	}
}

type probeResponse struct {
	status int
	body   string
}

func assertCapacity(t *testing.T, capacity Capacity, testCase struct {
	name                string
	responses           map[string]probeResponse
	registry            string
	model               string
	maxContext          int
	maxTokens           int
	wantSource          string
	wantContext         int
	wantCompletion      int
	wantSlots           int
	wantFits            *bool
	wantMessage         string
	checkRegistryCached bool
},
) {
	t.Helper()

	if capacity.Source != testCase.wantSource {
		t.Errorf("source = %q, want %q", capacity.Source, testCase.wantSource)
	}

	if pointerValue(capacity.ContextPerRequest) != testCase.wantContext {
		t.Errorf("context = %d, want %d", pointerValue(capacity.ContextPerRequest), testCase.wantContext)
	}

	if pointerValue(capacity.MaxCompletionTokens) != testCase.wantCompletion {
		t.Errorf(
			"max completion = %d, want %d",
			pointerValue(capacity.MaxCompletionTokens),
			testCase.wantCompletion,
		)
	}

	if pointerValue(capacity.Slots) != testCase.wantSlots {
		t.Errorf("slots = %d, want %d", pointerValue(capacity.Slots), testCase.wantSlots)
	}

	if !equalBoolPointers(capacity.Fits, testCase.wantFits) {
		t.Errorf("fits = %v, want %v", capacity.Fits, testCase.wantFits)
	}

	if capacity.Message != testCase.wantMessage {
		t.Errorf("message = %q, want %q", capacity.Message, testCase.wantMessage)
	}
}

func pointerValue(value *int) int {
	if value == nil {
		return 0
	}

	return *value
}

func boolPointer(value bool) *bool {
	return &value
}

func equalBoolPointers(left *bool, right *bool) bool {
	if left == nil || right == nil {
		return left == right
	}

	return *left == *right
}
