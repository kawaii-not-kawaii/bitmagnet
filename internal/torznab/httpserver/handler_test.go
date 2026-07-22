package httpserver_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/auth"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/torznab"
	"github.com/bitmagnet-io/bitmagnet/internal/torznab/httpserver"
	torznab_mocks "github.com/bitmagnet-io/bitmagnet/internal/torznab/mocks"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var testCfg = torznab.Config{
	Profiles: []torznab.Profile{
		{
			ID:           "test",
			Title:        "Test",
			DefaultLimit: 1000,
			MaxLimit:     2000,
			Tags:         []string{"test"},
		},
	},
}.MergeDefaults()

type testHarness struct {
	t                *testing.T
	clientMock       *torznab_mocks.Client
	responseRecorder *httptest.ResponseRecorder
	engine           *gin.Engine
	config           *concurrency.AtomicValue[torznab.Config]
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()
	return newTestHarnessWithAuth(t, auth.Config{Disabled: true})
}

func newTestHarnessWithAuth(t *testing.T, authConfig auth.Config) *testHarness {
	t.Helper()

	clientMock := torznab_mocks.NewClient(t)
	lazyClient := lazy.New[torznab.Client](func() (torznab.Client, error) {
		return clientMock, nil
	})

	cfg := &concurrency.AtomicValue[torznab.Config]{}
	cfg.Set(testCfg)
	authenticator, err := auth.NewAuthenticator(
		authConfig,
		configwrite.TargetPath(filepath.Join(t.TempDir(), "config.yml")),
		zap.NewNop().Sugar(),
	)
	require.NoError(t, err)

	engine := gin.New()
	err = httpserver.New(lazyClient, cfg, authenticator).Apply(engine)
	require.NoError(t, err)

	return &testHarness{
		t:                t,
		clientMock:       clientMock,
		responseRecorder: httptest.NewRecorder(),
		engine:           engine,
		config:           cfg,
	}
}

func TestCaps(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		url     string
		profile torznab.Profile
	}{
		{
			url:     "/torznab/?t=caps",
			profile: torznab.ProfileDefault,
		},
		{
			url:     "/torznab/api?t=caps",
			profile: torznab.ProfileDefault,
		},
		{
			url:     "/torznab/test?t=caps",
			profile: testCfg.Profiles[0],
		},
		{
			url:     "/torznab/test/api?t=caps",
			profile: testCfg.Profiles[0],
		},
		{
			url:     "/torznab/test/api/?t=caps",
			profile: testCfg.Profiles[0],
		},
	} {
		t.Run(testCase.url, func(t *testing.T) {
			t.Parallel()

			h := newTestHarness(t)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCase.url, nil)
			require.NoError(t, err)

			h.engine.ServeHTTP(h.responseRecorder, req)

			assert.Equal(t, http.StatusOK, h.responseRecorder.Code)
			assert.Equal(
				t,
				"application/xml; charset=utf-8",
				h.responseRecorder.Header().Get("Content-Type"),
			)

			expectedXML, err := testCase.profile.Caps().XML()
			require.NoError(t, err)
			assert.Equal(t, string(expectedXML), h.responseRecorder.Body.String())
		})
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		url             string
		expectedRequest torznab.SearchRequest
	}{
		{
			url: fmt.Sprintf("/torznab/?%s", url.Values{
				torznab.ParamType: []string{torznab.FunctionSearch},
			}.Encode()),
			expectedRequest: torznab.SearchRequest{
				Profile: torznab.ProfileDefault,
				Type:    torznab.FunctionSearch,
			},
		},
		{
			url: fmt.Sprintf("/torznab/?%s", url.Values{
				torznab.ParamType: []string{torznab.FunctionMovie},
				torznab.ParamCat: []string{
					strings.Join([]string{"2000", "2030"}, ","),
				},
				torznab.ParamLimit:  []string{"10"},
				torznab.ParamOffset: []string{"100"},
			}.Encode()),
			expectedRequest: torznab.SearchRequest{
				Profile: torznab.ProfileDefault,
				Type:    torznab.FunctionMovie,
				Cats:    []int{2000, 2030},
				Limit:   model.NewNullUint(10),
				Offset:  model.NewNullUint(100),
			},
		},
		{
			url: fmt.Sprintf("/torznab/%s?%s", testCfg.Profiles[0].ID, url.Values{
				torznab.ParamType: []string{torznab.FunctionSearch},
			}.Encode()),
			expectedRequest: torznab.SearchRequest{
				Profile: testCfg.Profiles[0],
				Type:    torznab.FunctionSearch,
			},
		},
		{
			url: fmt.Sprintf("/torznab/%s?%s", torznab.ProfileDefault.ID, url.Values{
				torznab.ParamType:   []string{torznab.FunctionTV},
				torznab.ParamIMDBID: []string{"123"},
				torznab.ParamSeason: []string{"1"},
			}.Encode()),
			expectedRequest: torznab.SearchRequest{
				Profile: torznab.ProfileDefault,
				Type:    torznab.FunctionTV,
				IMDBID:  model.NewNullString("123"),
				Season:  model.NewNullInt(1),
			},
		},
		{
			url: fmt.Sprintf("/torznab/%s?%s", torznab.ProfileDefault.ID, url.Values{
				torznab.ParamType:    []string{torznab.FunctionTV},
				torznab.ParamTMDBID:  []string{"123"},
				torznab.ParamSeason:  []string{"2"},
				torznab.ParamEpisode: []string{"3"},
			}.Encode()),
			expectedRequest: torznab.SearchRequest{
				Profile: torznab.ProfileDefault,
				Type:    torznab.FunctionTV,
				TMDBID:  model.NewNullString("123"),
				Season:  model.NewNullInt(2),
				Episode: model.NewNullInt(3),
			},
		},
	} {
		t.Run(testCase.url, func(t *testing.T) {
			t.Parallel()

			h := newTestHarness(t)

			result := torznab.SearchResult{}
			h.clientMock.EXPECT().Search(
				mock.IsType(&gin.Context{}),
				testCase.expectedRequest,
			).Return(result, nil).Times(1)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCase.url, nil)
			require.NoError(t, err)

			h.engine.ServeHTTP(h.responseRecorder, req)

			assert.Equal(t, http.StatusOK, h.responseRecorder.Code)

			resultXML, err := result.XML()
			require.NoError(t, err)
			assert.Equal(t, string(resultXML), h.responseRecorder.Body.String())
		})
	}
}

// TestLiveApply_ProfileAddedAtRuntime is the torznab live-apply guarantee: the
// handler reads the config behind the AtomicValue per request, so a profile
// added by a runtime config update is served without a restart — and it is
// default-merged at use-time, so a raw (unmerged) value set by a future config
// mutation still gets defaults applied.
func TestLiveApply_ProfileAddedAtRuntime(t *testing.T) {
	t.Parallel()

	h := newTestHarness(t)

	// The profile does not exist at startup.
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/torznab/added/api?t=caps",
		nil,
	)
	require.NoError(t, err)

	h.engine.ServeHTTP(h.responseRecorder, req)
	assert.Equal(t, http.StatusNotFound, h.responseRecorder.Code)

	// Add it at runtime — deliberately WITHOUT MergeDefaults, as a raw value
	// from a config mutation would arrive.
	h.config.Set(torznab.Config{
		Profiles: []torznab.Profile{
			{ID: "added", Title: "Added at runtime"},
		},
	})

	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Use-time MergeDefaults filled the limits the raw profile omitted.
	expected := torznab.Profile{ID: "added", Title: "Added at runtime"}.MergeDefaults()
	expectedXML, err := expected.Caps().XML()
	require.NoError(t, err)
	assert.Equal(t, string(expectedXML), rec.Body.String())
}
