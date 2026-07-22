package httpserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/bitmagnet-io/bitmagnet/internal/gql/auth"
	"github.com/bitmagnet-io/bitmagnet/internal/torznab"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTorznabAuthAcceptsQueryAndHeaderKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		path   string
		header string
	}{
		{name: "query", path: "/torznab/?t=caps&apikey=machine-key"},
		{name: "header", path: "/torznab/?t=caps", header: "machine-key"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newTestHarnessWithAuth(t, auth.Config{APIKey: "machine-key"})
			request, err := http.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				testCase.path,
				nil,
			)
			require.NoError(t, err)
			if testCase.header != "" {
				request.Header.Set("X-Api-Key", testCase.header)
			}

			harness.engine.ServeHTTP(harness.responseRecorder, request)
			assert.Equal(t, http.StatusOK, harness.responseRecorder.Code)
		})
	}
}

func TestTorznabAuthRejectsMissingWrongAndTrustedNetwork(t *testing.T) {
	t.Parallel()
	expected, err := (torznab.Error{Code: 100, Description: "Incorrect user credentials"}).XML()
	require.NoError(t, err)

	cases := []struct {
		name       string
		path       string
		remoteAddr string
	}{
		{name: "missing", path: "/torznab/?t=caps", remoteAddr: "203.0.113.10:1234"},
		{name: "wrong", path: "/torznab/?t=caps&apikey=wrong", remoteAddr: "203.0.113.10:1234"},
		{name: "trusted-network", path: "/torznab/?t=caps", remoteAddr: "10.1.2.3:1234"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newTestHarnessWithAuth(t, auth.Config{
				APIKey: "machine-key", TrustedNetworks: []string{"10.0.0.0/8"},
			})
			request, requestErr := http.NewRequestWithContext(
				context.Background(), http.MethodGet, testCase.path, nil,
			)
			require.NoError(t, requestErr)
			request.RemoteAddr = testCase.remoteAddr

			harness.engine.ServeHTTP(harness.responseRecorder, request)
			assert.Equal(t, http.StatusUnauthorized, harness.responseRecorder.Code)
			assert.Equal(
				t,
				"application/xml; charset=utf-8",
				harness.responseRecorder.Header().Get("Content-Type"),
			)
			assert.Equal(t, string(expected), harness.responseRecorder.Body.String())
		})
	}
}

func TestTorznabAuthDisabledPassesThrough(t *testing.T) {
	t.Parallel()
	harness := newTestHarnessWithAuth(t, auth.Config{Disabled: true})
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/torznab/?t=caps", nil)
	require.NoError(t, err)

	harness.engine.ServeHTTP(harness.responseRecorder, request)
	assert.Equal(t, http.StatusOK, harness.responseRecorder.Code)
}
