package resolvers

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/config"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configapply"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/auth"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
	"github.com/bitmagnet-io/bitmagnet/internal/tmdb"
	"github.com/go-playground/validator/v10"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func TestConfigMutation_SetSectionRedactsAndRefreshesQuery(t *testing.T) {
	t.Parallel()

	resolver, applied := newConfigMutationTestResolver(t)
	mutation := &configMutationResolver{resolver}
	want := tmdb.Config{
		Enabled:        false,
		BaseURL:        "https://tmdb.example.com",
		APIKey:         "new-secret",
		RateLimit:      2 * time.Second,
		RateLimitBurst: 8,
	}

	result, err := mutation.SetSection(adminContext(), &gen.ConfigMutation{}, gen.SetConfigSectionInput{
		Key: testSectionKeyTmdb,
		Value: map[string]any{
			"Enabled":        want.Enabled,
			"BaseURL":        want.BaseURL,
			"APIKey":         want.APIKey,
			"RateLimit":      "2s",
			"RateLimitBurst": float64(want.RateLimitBurst),
		},
	})
	if err != nil {
		t.Fatalf("SetSection: %v", err)
	}

	if result.Applied != gen.ConfigRuntimeChangeabilityLiveApplyAvailable {
		t.Errorf("applied = %v, want LIVE_APPLY_AVAILABLE", result.Applied)
	}

	if result.Section.Key != testSectionKeyTmdb {
		t.Errorf("section key = %q, want %q", result.Section.Key, testSectionKeyTmdb)
	}

	if result.Section.RuntimeChangeable != gen.ConfigRuntimeChangeabilityLiveApplyAvailable {
		t.Errorf("runtimeChangeable = %v, want LIVE_APPLY_AVAILABLE", result.Section.RuntimeChangeable)
	}

	if !reflect.DeepEqual(*applied, want) {
		t.Errorf("live-applied config = %#v, want %#v", *applied, want)
	}

	mutationValue, ok := result.Section.Value.(map[string]any)
	if !ok {
		t.Fatalf("mutation section value has type %T", result.Section.Value)
	}

	if mutationValue["APIKey"] != gqlmodel.RedactedValuePlaceholder {
		t.Errorf("mutation echoed API key: %v", mutationValue["APIKey"])
	}

	if strings.Contains(fmt.Sprint(mutationValue), want.APIKey) {
		t.Fatal("mutation response contains submitted secret")
	}

	queryResult, err := (&queryResolver{resolver}).Config(context.Background())
	if err != nil {
		t.Fatalf("follow-up Config query: %v", err)
	}

	if len(queryResult.Sections) != 2 {
		t.Fatalf("follow-up Config sections = %d, want 2", len(queryResult.Sections))
	}

	var tmdbSection gen.ConfigSection

	for _, section := range queryResult.Sections {
		if section.Key == testSectionKeyTmdb {
			tmdbSection = section
			break
		}
	}

	queryValue, ok := tmdbSection.Value.(map[string]any)
	if !ok {
		t.Fatalf("follow-up tmdb value has type %T", tmdbSection.Value)
	}

	if queryValue["BaseURL"] != want.BaseURL {
		t.Errorf("follow-up BaseURL = %v, want %v", queryValue["BaseURL"], want.BaseURL)
	}

	if queryValue["APIKey"] != gqlmodel.RedactedValuePlaceholder {
		t.Errorf("follow-up query echoed API key: %v", queryValue["APIKey"])
	}
}

func TestConfigMutation_SectionErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "unknown", key: "missing", want: `unknown config section "missing"`},
		{name: "denied", key: testSectionKeyPostgres, want: `config section "postgres" is not mutable via API`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver, _ := newConfigMutationTestResolver(t)
			_, err := (&configMutationResolver{resolver}).SetSection(
				adminContext(),
				&gen.ConfigMutation{},
				gen.SetConfigSectionInput{Key: tt.key, Value: map[string]any{}},
			)

			if err == nil {
				t.Fatal("SetSection unexpectedly succeeded")
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("SetSection error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestConfigMutation_Authorization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ctx       context.Context
		forbidden bool
	}{
		{name: "absent principal", ctx: context.Background(), forbidden: true},
		{name: "admin", ctx: adminContext()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver, _ := newConfigMutationTestResolver(t)
			_, err := (&configMutationResolver{resolver}).SetSection(
				tt.ctx,
				&gen.ConfigMutation{},
				validTmdbInput(),
			)

			if !tt.forbidden {
				if err != nil {
					t.Fatalf("admin SetSection: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatal("SetSection without principal unexpectedly succeeded")
			}

			var gqlErr *gqlerror.Error
			if !errors.As(err, &gqlErr) {
				t.Fatalf("authorization error has type %T", err)
			}

			if gqlErr.Extensions["code"] != "FORBIDDEN" {
				t.Fatalf("authorization code = %v, want FORBIDDEN", gqlErr.Extensions["code"])
			}
		})
	}
}

func newConfigMutationTestResolver(t *testing.T) (*Resolver, *tmdb.Config) {
	t.Helper()

	initial := tmdb.Config{
		Enabled:        true,
		BaseURL:        "https://api.themoviedb.org/3",
		APIKey:         "old-secret",
		RateLimit:      time.Second,
		RateLimitBurst: 5,
	}
	resolved := liveResolved(config.ResolvedConfig{NodeMap: map[string]config.ResolvedNode{
		testSectionKeyTmdb: {
			Spec:     config.Spec{Key: testSectionKeyTmdb, DefaultValue: initial},
			IsStruct: true,
			Type:     reflect.TypeOf(initial),
			Value:    initial,
			ValueRaw: initial,
		},
		testSectionKeyPostgres: {
			Spec:     config.Spec{Key: testSectionKeyPostgres, DefaultValue: initial},
			IsStruct: true,
			Type:     reflect.TypeOf(initial),
			Value:    initial,
			ValueRaw: initial,
		},
	}})

	applied := initial
	result := configapply.New(configapply.Params{
		Appliers: []configapply.LiveApplier{{
			Key: testSectionKeyTmdb,
			Apply: func(value any) (func(), error) {
				applied = value.(tmdb.Config)

				return func() {}, nil
			},
		}},
		Resolved: resolved,
		Validate: validator.New(),
		Path:     configwrite.TargetPath(filepath.Join(t.TempDir(), "config.yml")),
	})

	return &Resolver{
		ResolvedConfig: resolved,
		Changeability:  result.Changeability,
		Applier:        result.Applier,
	}, &applied
}

func adminContext() context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{AccessLevel: auth.AccessLevelAdmin})
}

func validTmdbInput() gen.SetConfigSectionInput {
	return gen.SetConfigSectionInput{
		Key: testSectionKeyTmdb,
		Value: map[string]any{
			"Enabled":        false,
			"BaseURL":        "https://tmdb.example.com",
			"APIKey":         "new-secret",
			"RateLimit":      "2s",
			"RateLimitBurst": float64(8),
		},
	}
}
