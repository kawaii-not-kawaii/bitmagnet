package classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/armiger"
	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// armigerTestTorrent is an unhinted torrent that no rule in the core workflow
// types, so the result is empty apart from whatever armiger_classify adds.
func armigerTestTorrent() model.Torrent {
	return model.Torrent{
		Name:        "Some Obscure Title",
		FilesStatus: model.FilesStatusMulti,
		Size:        123456,
		Files: []model.TorrentFile{
			{Path: "Some Obscure Title/data.bin", Size: 100000},
			{Path: "Some Obscure Title/info.nfo", Size: 23456},
		},
	}
}

// runArmigerWorkflow compiles the core workflow with the given armiger
// config and runs it for the torrent.
func runArmigerWorkflow(
	t *testing.T,
	cfg armiger.Config,
	torrent model.Torrent,
) (classification.Result, error) {
	t.Helper()

	mocks := newTestClassifierMocks(t)

	c, ok := mocks.compiler.(compiler)
	require.True(t, ok)

	c.dependencies.armiger = armiger.NewClient(cfg)
	c.dependencies.logger = zap.NewNop().Sugar()

	source, err := coreSourceProvider{}.provider().source()
	require.NoError(t, err)

	workflow, err := c.Compile(source)
	require.NoError(t, err)

	return workflow.Run(context.Background(), "default", Flags{}, torrent)
}

func armigerServer(t *testing.T, calls *atomic.Int32, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func armigerResponse(category string, confidence float64) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"category": category,
			"adult":    map[string]any{"label": "safe"},
			"heads": map[string]any{
				"category": map[string]any{"confidence": confidence, "label": category},
			},
		})
	}
}

func tagsOf(r classification.Result) map[string]struct{} {
	if r.Tags == nil {
		return nil
	}

	return r.Tags.Add
}

func TestArmigerClassify_TagsOnly(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		category   string
		confidence float64
		wantTags   []string
	}{
		{"confident tv show", "tv_show", 0.93, []string{"armiger-tv-show", "armiger-conf-90"}},
		{"low confidence movie", "movie", 0.37, []string{"armiger-movie", "armiger-conf-30"}},
		{"zero confidence", "game", 0.0, []string{"armiger-game", "armiger-conf-0"}},
		{"exact tenth", "music", 0.7, []string{"armiger-music", "armiger-conf-70"}},
		{"full confidence", "xxx", 1.0, []string{"armiger-xxx", "armiger-conf-90"}},
		{"unknown", "unknown", 0.5, []string{"armiger-unknown", "armiger-conf-50"}},
		{"unrecognised category", "something_new", 0.8, []string{"armiger-unknown", "armiger-conf-80"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32

			srv := armigerServer(t, &calls, armigerResponse(tc.category, tc.confidence))

			got, err := runArmigerWorkflow(t, armiger.Config{
				Enabled: true, URL: srv.URL, Timeout: time.Second,
			}, armigerTestTorrent())
			require.NoError(t, err)

			baseline, err := runArmigerWorkflow(t, armiger.Config{}, armigerTestTorrent())
			require.NoError(t, err)

			want := map[string]struct{}{}
			for _, tag := range tc.wantTags {
				want[tag] = struct{}{}
			}

			assert.Equal(t, want, tagsOf(got))
			assert.Equal(t, int32(1), calls.Load())

			// Nothing but tags may differ from the run with armiger disabled.
			got.Tags = nil
			assert.Equal(t, baseline, got)
		})
	}
}

func TestArmigerClassify_FailuresLeaveResultUnchanged(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		handler http.HandlerFunc
		timeout time.Duration
	}{
		{
			name: "server error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			},
			timeout: time.Second,
		},
		{
			name: "malformed json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("{not json"))
			},
			timeout: time.Second,
		},
		{
			name: "missing confidence head",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"category":"movie"}`))
			},
			timeout: time.Second,
		},
		{
			name: "timeout",
			handler: func(_ http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-time.After(5 * time.Second):
				}
			},
			timeout: 50 * time.Millisecond,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32

			srv := armigerServer(t, &calls, tc.handler)

			got, err := runArmigerWorkflow(t, armiger.Config{
				Enabled: true, URL: srv.URL, Timeout: tc.timeout,
			}, armigerTestTorrent())
			require.NoError(t, err)

			baseline, err := runArmigerWorkflow(t, armiger.Config{}, armigerTestTorrent())
			require.NoError(t, err)

			assert.Equal(t, int32(1), calls.Load())
			assert.Nil(t, got.Tags)
			assert.Equal(t, baseline, got)
		})
	}
}

func TestArmigerClassify_UnreachableServerLeavesResultUnchanged(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	srv := armigerServer(t, &calls, armigerResponse("movie", 0.9))
	url := srv.URL
	srv.Close()

	got, err := runArmigerWorkflow(t, armiger.Config{
		Enabled: true, URL: url, Timeout: time.Second,
	}, armigerTestTorrent())
	require.NoError(t, err)
	assert.Nil(t, got.Tags)
}

func TestArmigerClassify_NotCalledWhenDisabledOrUnconfigured(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	srv := armigerServer(t, &calls, armigerResponse("movie", 0.9))

	got, err := runArmigerWorkflow(t, armiger.Config{
		Enabled: false, URL: srv.URL, Timeout: time.Second,
	}, armigerTestTorrent())
	require.NoError(t, err)
	assert.Nil(t, got.Tags)

	got, err = runArmigerWorkflow(t, armiger.Config{
		Enabled: true, URL: "", Timeout: time.Second,
	}, armigerTestTorrent())
	require.NoError(t, err)
	assert.Nil(t, got.Tags)

	assert.Equal(t, int32(0), calls.Load())
}

func TestArmigerClassify_NotCalledForHintedContentType(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	srv := armigerServer(t, &calls, armigerResponse("tv_show", 0.9))

	torrent := armigerTestTorrent()
	torrent.Hint = model.TorrentHint{ContentType: model.ContentTypeSoftware}

	got, err := runArmigerWorkflow(t, armiger.Config{
		Enabled: true, URL: srv.URL, Timeout: time.Second,
	}, torrent)
	require.NoError(t, err)

	assert.Equal(t, int32(0), calls.Load())
	assert.Nil(t, got.Tags)
	assert.Equal(t, model.NewNullContentType(model.ContentTypeSoftware), got.ContentType)
}

func TestArmigerClassify_RequestCarriesNameSizeAndFiles(t *testing.T) {
	t.Parallel()

	var (
		calls atomic.Int32
		path  atomic.Value
		body  atomic.Value
		ctype atomic.Value
	)

	srv := armigerServer(t, &calls, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		path.Store(r.Method + " " + r.URL.Path)
		body.Store(raw)
		ctype.Store(r.Header.Get("Content-Type"))
		armigerResponse("movie", 0.5)(w, r)
	})

	// A trailing slash on the configured URL must not break the path.
	_, err := runArmigerWorkflow(t, armiger.Config{
		Enabled: true, URL: srv.URL + "/", Timeout: time.Second,
	}, armigerTestTorrent())
	require.NoError(t, err)

	require.Equal(t, int32(1), calls.Load())
	assert.Equal(t, "POST /v1/classify", path.Load())
	assert.Equal(t, "application/json", ctype.Load())

	raw, ok := body.Load().([]byte)
	require.True(t, ok)
	assert.JSONEq(t, `{
		"name": "Some Obscure Title",
		"size": 123456,
		"files": [
			{"path": "Some Obscure Title/data.bin", "size": 100000},
			{"path": "Some Obscure Title/info.nfo", "size": 23456}
		]
	}`, string(raw))
}

// capturedArmigerRequest runs the workflow for torrent and returns the decoded
// request body armiger received.
func capturedArmigerRequest(t *testing.T, torrent model.Torrent) map[string]any {
	t.Helper()

	var (
		calls atomic.Int32
		body  atomic.Value
	)

	srv := armigerServer(t, &calls, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body.Store(raw)
		armigerResponse("movie", 0.5)(w, r)
	})

	_, err := runArmigerWorkflow(t, armiger.Config{Enabled: true, URL: srv.URL, Timeout: time.Second}, torrent)
	require.NoError(t, err)
	require.Equal(t, int32(1), calls.Load())

	raw, ok := body.Load().([]byte)
	require.True(t, ok)

	req := map[string]any{}
	require.NoError(t, json.Unmarshal(raw, &req))

	return req
}

// Single-file torrents have no file rows; armiger gets the torrent itself as
// its one file, which is the input the labeled-set evaluation used.
func TestArmigerClassify_SingleFileTorrentSendsItselfAsFile(t *testing.T) {
	t.Parallel()

	req := capturedArmigerRequest(t, model.Torrent{
		Name:        "Some Obscure Title.mkv",
		FilesStatus: model.FilesStatusSingle,
		Size:        5000,
	})

	assert.Equal(t, []any{map[string]any{"path": "Some Obscure Title.mkv", "size": float64(5000)}}, req["files"])
}

// Very large torrents must not send every file on every classification.
func TestArmigerClassify_FileListIsCapped(t *testing.T) {
	t.Parallel()

	torrent := armigerTestTorrent()
	torrent.Files = make([]model.TorrentFile, armigerMaxFiles+50)

	for i := range torrent.Files {
		torrent.Files[i] = model.TorrentFile{Path: fmt.Sprintf("dir/file%03d.bin", i), Size: 1}
	}

	req := capturedArmigerRequest(t, torrent)

	files, ok := req["files"].([]any)
	require.True(t, ok)
	assert.Len(t, files, armigerMaxFiles)
}
