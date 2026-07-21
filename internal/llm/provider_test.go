package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// mockProvider implements Provider for testing.
type mockProvider struct {
	name       string
	result     *ClassifyResult
	err        error
	calledWith []ClassifyInput
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Classify(_ context.Context, input ClassifyInput) (*ClassifyResult, error) {
	m.calledWith = append(m.calledWith, input)
	return m.result, m.err
}

func (m *mockProvider) BatchClassify(_ context.Context, inputs []ClassifyInput) ([]*ClassifyResult, error) {
	m.calledWith = append(m.calledWith, inputs...)

	if m.err != nil {
		return nil, m.err
	}

	results := make([]*ClassifyResult, len(inputs))

	for i := range inputs {
		if m.result != nil {
			r := *m.result
			results[i] = &r
		}
	}

	return results, nil
}

func TestClassifyInput_ShallowCopySemantics(t *testing.T) {
	t.Parallel()

	original := ClassifyInput{Name: "Test.Movie.2024.1080p", Files: []string{"file1.mkv", "file2.nfo"}}
	copied := original
	copied.Name = "Changed"
	copied.Files[0] = "changed.mkv"

	// Name is a string — copied independently on struct assignment.
	if original.Name != "Test.Movie.2024.1080p" {
		t.Error("Name should be independent after struct copy")
	}

	// Files is a slice — backing array is SHARED after struct copy. This is
	// intentional and load-bearing: the batch path (internal/llm/openai/batch.go)
	// stores ClassifyInput by value in pendingRequest.input, and a later flush
	// goroutine reads input.Files from that stored copy to build the HTTP
	// request body. If callers ever mutate input.Files after handing it to a
	// Provider, that mutation will be visible to the flush goroutine racing
	// against the caller.
	//
	// The contract this test pins down: struct copy is shallow on the slice
	// header. If anyone ever makes Files deep-copy (e.g. by changing the type
	// to a value-type wrapper, or by adding Clone() calls at handoff), this
	// assertion will fail and force them to audit the batch path.
	if original.Files[0] != "changed.mkv" {
		t.Error("Files backing array should be shared after struct copy; " +
			"if this fails, audit internal/llm/openai/batch.go pendingRequest " +
			"callers — they rely on the shallow-copy contract and must be " +
			"updated together with any deep-copy change.")
	}
}

func TestClassifyInput_EmptyFiles(t *testing.T) {
	t.Parallel()

	input := ClassifyInput{Name: "Some.Torrent"}
	if input.Files != nil {
		t.Error("expected nil files when not set")
	}

	if len(input.Files) != 0 {
		t.Error("expected empty files")
	}
}

func TestProvider_Interface(t *testing.T) {
	t.Parallel()

	expected := &ClassifyResult{
		ContentType: "movie",
		Title:       "Test Movie",
		Year:        2024,
	}
	mock := &mockProvider{name: "test", result: expected}

	var p Provider = mock

	result, err := p.Classify(context.Background(), ClassifyInput{Name: "Test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "Test Movie" {
		t.Errorf("expected 'Test Movie', got %q", result.Title)
	}

	if result.Year != 2024 {
		t.Errorf("expected 2024, got %d", result.Year)
	}
}

func TestProvider_Error(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("connection refused")
	mock := &mockProvider{name: "test", err: expectedErr}

	var p Provider = mock

	_, err := p.Classify(context.Background(), ClassifyInput{Name: "Test"})
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestBatchProvider_Interface(t *testing.T) {
	t.Parallel()

	expected := &ClassifyResult{
		ContentType:     "tv_show",
		Title:           "Test Show",
		Season:          1,
		Episode:         5,
		VideoResolution: "V1080p",
	}
	mock := &mockProvider{name: "test", result: expected}

	var bp BatchProvider = mock

	inputs := []ClassifyInput{
		{Name: "Show.S01E01"},
		{Name: "Show.S01E02"},
	}

	results, err := bp.BatchClassify(context.Background(), inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for i, r := range results {
		if r.ContentType != "tv_show" {
			t.Errorf("result[%d] content_type = %q, expected tv_show", i, r.ContentType)
		}
	}
}

func TestBatchProvider_Fallback(t *testing.T) {
	t.Parallel()

	mock := &mockProvider{name: "test", result: &ClassifyResult{ContentType: "movie"}}

	var p Provider = mock

	bp, ok := p.(BatchProvider)
	if !ok {
		t.Fatal("mockProvider should implement BatchProvider")
	}

	results, err := bp.BatchClassify(context.Background(), []ClassifyInput{{Name: "A"}, {Name: "B"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestClassifyResult_JSON(t *testing.T) {
	t.Parallel()

	r := ClassifyResult{
		ContentType:     "movie",
		Title:           "Test",
		Year:            2024,
		VideoResolution: "V1080p",
		Tags:            []string{"action", "sci-fi"},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ClassifyResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ContentType != "movie" {
		t.Errorf("expected movie, got %q", decoded.ContentType)
	}

	if decoded.Year != 2024 {
		t.Errorf("expected 2024, got %d", decoded.Year)
	}

	if len(decoded.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(decoded.Tags))
	}
}

func TestClassifyResult_JSON_ZeroValues(t *testing.T) {
	t.Parallel()

	r := ClassifyResult{
		ContentType: "music",
		Title:       "Album",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ClassifyResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Zero values should be 0 / ""
	if decoded.Year != 0 {
		t.Errorf("expected 0 year for zero value, got %d", decoded.Year)
	}

	if decoded.VideoResolution != "" {
		t.Errorf("expected empty resolution, got %q", decoded.VideoResolution)
	}
}

func TestProviderName(t *testing.T) {
	t.Parallel()

	mock := &mockProvider{name: "gemma4"}
	if mock.Name() != "gemma4" {
		t.Errorf("expected gemma4, got %q", mock.Name())
	}
}

func TestClassifyInput_ContentTypes(t *testing.T) {
	t.Parallel()

	input := ClassifyInput{
		Name:         "Test",
		ContentTypes: "movie, tv_show, music",
	}

	if input.ContentTypes != "movie, tv_show, music" {
		t.Errorf("expected content types to be preserved")
	}
}

func TestBatchProvider_Error(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("timeout")
	mock := &mockProvider{name: "test", err: expectedErr}
	bp := mock

	_, err := bp.BatchClassify(context.Background(), []ClassifyInput{{Name: "A"}})
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestErrorSentinel(t *testing.T) {
	t.Parallel()

	if ErrNoResult == nil {
		t.Error("ErrNoResult should not be nil")
	}

	if ErrInvalidJSON == nil {
		t.Error("ErrInvalidJSON should not be nil")
	}

	if errors.Is(ErrNoResult, ErrInvalidJSON) {
		t.Error("error sentinels should be distinct")
	}
}

// TestClassifyResult_FlexInt_Coercion locks in the type-coercion contract for
// Year/Season/Episode: the LLM often returns VALID JSON with the WRONG types
// for these fields (strings for episode numbers, arrays for multi-episode
// packs, etc.), and the provider layer must tolerate them. A bare int and a
// missing field are the baseline; the rest are the coercions documented on
// flexInt. See TestClassifyResult_MalformedStillErrors for the negative side.
func TestClassifyResult_FlexInt_Coercion(t *testing.T) {
	t.Parallel()

	type expectation struct {
		year    int
		season  int
		episode int
	}

	cases := []struct {
		name string
		body string
		want expectation
	}{
		// --- Year (representative of all three numeric fields) ---
		{ name: "year/number", body: `{"content_type":"movie","year":2024}`, want: expectation{year: 2024} },
		{ name: "year/numeric_string", body: `{"content_type":"movie","year":"2024"}`, want: expectation{year: 2024} },
		{ name: "year/empty_string", body: `{"content_type":"movie","year":""}`, want: expectation{} },
		{ name: "year/non_numeric_string", body: `{"content_type":"movie","year":"unknown"}`, want: expectation{} },
		{ name: "year/array_first_element", body: `{"content_type":"movie","year":[2024,2025]}`, want: expectation{year: 2024} },
		{ name: "year/empty_array", body: `{"content_type":"movie","year":[]}`, want: expectation{} },
		{ name: "year/null", body: `{"content_type":"movie","year":null}`, want: expectation{} },
		{ name: "year/missing", body: `{"content_type":"movie"}`, want: expectation{} },

		// --- Year: floats (LLMs often emit 2024.0 for integer fields) ---
		// All truncate toward zero, matching Go's int(float64) conversion.
		{ name: "year/float_integral_zero_fraction", body: `{"content_type":"movie","year":2024.0}`, want: expectation{year: 2024} },
		{ name: "year/float_small_integral", body: `{"content_type":"movie","year":3.0}`, want: expectation{year: 3} },
		{ name: "year/float_non_integral_truncates", body: `{"content_type":"movie","year":2024.7}`, want: expectation{year: 2024} },
		{ name: "year/float_negative_truncates_toward_zero", body: `{"content_type":"movie","year":-1.5}`, want: expectation{year: -1} },
		{ name: "year/float_large", body: `{"content_type":"movie","year":10000000000.0}`, want: expectation{year: 10000000000} },
		{ name: "year/float_in_string", body: `{"content_type":"movie","year":"2024.0"}`, want: expectation{year: 2024} },

		// --- Episode (the field most often emitted with the wrong type) ---
		{ name: "episode/number", body: `{"content_type":"tv_show","episode":5}`, want: expectation{episode: 5} },
		{ name: "episode/numeric_string_quoted", body: `{"content_type":"tv_show","episode":"5"}`, want: expectation{episode: 5} },
		{ name: "episode/empty_string", body: `{"content_type":"tv_show","episode":""}`, want: expectation{} },
		{ name: "episode/non_numeric_string", body: `{"content_type":"tv_show","episode":"pilot"}`, want: expectation{} },
		{ name: "episode/multi_ep_array_takes_first", body: `{"content_type":"tv_show","episode":[1,2,3]}`, want: expectation{episode: 1} },
		{ name: "episode/single_element_array", body: `{"content_type":"tv_show","episode":[7]}`, want: expectation{episode: 7} },
		{ name: "episode/array_of_strings", body: `{"content_type":"tv_show","episode":["7","8"]}`, want: expectation{episode: 7} },
		{ name: "episode/null", body: `{"content_type":"tv_show","episode":null}`, want: expectation{} },
		{ name: "episode/missing", body: `{"content_type":"tv_show"}`, want: expectation{} },

		// --- Season (same shape; one coercion case is enough since the path is shared) ---
		{ name: "season/numeric_string", body: `{"content_type":"tv_show","season":"2"}`, want: expectation{season: 2} },
		{ name: "season/array", body: `{"content_type":"tv_show","season":[2,3]}`, want: expectation{season: 2} },

		// --- Combined real-world failure cases from the live benchmark ---
		{ name: "real_world/anime_episode_string", body: `{"content_type":"tv_show","title":"BGC Tokyo 2040","season":1,"episode":"26"}`, want: expectation{season: 1, episode: 26} },
		{ name: "real_world/culpa_array_case", body: `{"content_type":"movie","title":"Culpa","year":[2022]}`, want: expectation{year: 2022} },
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var r ClassifyResult
			if err := json.Unmarshal([]byte(tc.body), &r); err != nil {
				t.Fatalf("unexpected error for %s: %v\nbody: %s", tc.name, err, tc.body)
			}
			if r.Year != tc.want.year {
				t.Errorf("Year: got %d, want %d", r.Year, tc.want.year)
			}
			if r.Season != tc.want.season {
				t.Errorf("Season: got %d, want %d", r.Season, tc.want.season)
			}
			if r.Episode != tc.want.episode {
				t.Errorf("Episode: got %d, want %d", r.Episode, tc.want.episode)
			}
		})
	}
}

// TestClassifyResult_MalformedStillErrors is the negative-side guard: only the
// specific numeric type-coercions documented on flexInt are tolerated (incl.
// floats, which truncate toward zero). Genuinely malformed input, wrong types
// for non-numeric fields, and objects for numeric fields must still surface as
// real errors so silent corruption cannot pass.
func TestClassifyResult_MalformedStillErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{ name: "broken_json_syntax", body: `{"content_type":"movie", broken` },
		{ name: "string_field_as_array", body: `{"content_type":["movie"]}` },                 // non-numeric field not made lenient
		{ name: "tags_field_as_object", body: `{"content_type":"movie","tags":{"a":1}}` },     // slice field given wrong shape
		{ name: "year_as_object", body: `{"content_type":"movie","year":{"value":2024}}` },    // object for numeric field
		{ name: "episode_as_object", body: `{"content_type":"tv_show","episode":{"n":5}}` },   // object for numeric field
		{ name: "root_array_not_object", body: `[{"content_type":"movie"}]` },                 // expected an object, got array
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var r ClassifyResult
			if err := json.Unmarshal([]byte(tc.body), &r); err == nil {
				t.Errorf("expected error for %s, got nil; result=%+v", tc.name, r)
			}
		})
	}
}
