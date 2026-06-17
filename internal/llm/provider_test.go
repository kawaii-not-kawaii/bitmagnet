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

func TestClassifyInput_ValueSemantics(t *testing.T) {
	t.Parallel()

	original := ClassifyInput{Name: "Test.Movie.2024.1080p", Files: []string{"file1.mkv", "file2.nfo"}}
	copied := original
	copied.Name = "Changed"
	copied.Files[0] = "changed.mkv"

	if original.Name != "Test.Movie.2024.1080p" {
		t.Error("Name was not independent after copy")
	}

	if original.Files[0] != "file1.mkv" {
		t.Error("Files slice was shared (slice header copy)")
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
