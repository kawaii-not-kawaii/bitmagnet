package resolvers

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/llm"
)

type dashboardTestProvider struct {
	err error
}

func (dashboardTestProvider) Name() string { return "test" }

func (p dashboardTestProvider) Classify(context.Context, llm.ClassifyInput) (*llm.ClassifyResult, error) {
	if p.err != nil {
		return nil, p.err
	}

	return &llm.ClassifyResult{ContentType: "movie"}, nil
}

func TestDashboardLlmConnection(t *testing.T) {
	t.Parallel()

	providerErr := errors.New("unauthorized")
	cases := []struct {
		name      string
		provider  dashboardTestProvider
		wantOK    bool
		wantError string
	}{
		{name: "connected", wantOK: true},
		{
			name:      "provider error",
			provider:  dashboardTestProvider{err: providerErr},
			wantError: providerErr.Error(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := llm.RegistryConfig{
				Enabled:   true,
				Providers: map[string]llm.ProviderConfig{"test": {}},
			}
			registry := llm.NewRegistry(
				config,
				func(string, llm.ProviderConfig, llm.RegistryConfig) llm.Provider { return tc.provider },
				"",
			)
			before := registry.Config()

			result, err := (&Resolver{LlmRegistry: registry}).testDashboardLlmConnection(
				context.Background(),
			)
			if err != nil {
				t.Fatalf("testDashboardLlmConnection: %v", err)
			}

			if result.Ok != tc.wantOK || result.Connected != tc.wantOK {
				t.Errorf(
					"connection flags = ok %t/connected %t, want %t",
					result.Ok,
					result.Connected,
					tc.wantOK,
				)
			}

			if tc.wantError == "" {
				if result.Error != nil {
					t.Errorf("error = %q, want nil", *result.Error)
				}
			} else if result.Error == nil || !strings.Contains(*result.Error, tc.wantError) {
				t.Errorf("error = %v, want message containing %q", result.Error, tc.wantError)
			}

			if !reflect.DeepEqual(registry.Config(), before) {
				t.Errorf("connection test changed registry config")
			}
		})
	}
}

func TestDashboardLlmConnectionWithoutProvider(t *testing.T) {
	t.Parallel()

	registry := llm.NewRegistry(llm.RegistryConfig{}, nil, "")
	result, err := (&Resolver{LlmRegistry: registry}).testDashboardLlmConnection(context.Background())
	if err != nil {
		t.Fatalf("testDashboardLlmConnection: %v", err)
	}

	if result.Ok || result.Error == nil {
		t.Fatalf("result = %#v, want failed result with message", result)
	}
}
