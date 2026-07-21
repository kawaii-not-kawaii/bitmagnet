package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/llm"
)

// testContentTypeMovie is the content_type value used across LLM response
// fixtures; extracted to satisfy goconst.
const testContentTypeMovie = "movie"

// makeChoiceResp returns a chatResponse with one choice whose content is set to the given string.
// It builds the value via JSON round-trip to avoid nested anonymous struct literals.
func makeChoiceResp(content string) chatResponse {
	raw := fmt.Sprintf(`{"choices":[{"message":{"content":%s},"finish_reason":""}]}`,
		string(mustMarshalString(content)))

	var resp chatResponse

	_ = json.Unmarshal([]byte(raw), &resp)

	return resp
}

func mustMarshalString(s string) []byte {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}

	return b
}

func TestName(t *testing.T) {
	t.Parallel()

	p := New(Config{Name: "gemma4"})

	if p.Name() != "gemma4" {
		t.Errorf("expected gemma4, got %q", p.Name())
	}
}

func TestClassify_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}

		// Verify auth header
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %q", r.Header.Get("Authorization"))
		}

		// Verify request body
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}

		if req.Model != "test-model" {
			t.Errorf("expected test-model, got %q", req.Model)
		}

		if req.Temperature != 0.1 {
			t.Errorf("expected temp 0.1, got %f", req.Temperature)
		}

		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
			t.Errorf("expected json_object response format")
		}

		resp := makeChoiceResp(fmt.Sprintf(`{"content_type": %q, "title": "Test Movie", "year": 2024}`, testContentTypeMovie))
		resp.Choices[0].FinishReason = "stop"

		w.Header().Set("Content-Type", "application/json")

		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New(Config{
		Name:    "test",
		BaseURL: srv.URL,
		Model:   "test-model",
		APIKey:  "test-key",
	})

	result, err := p.Classify(context.Background(), llm.ClassifyInput{Name: "Test.Movie.2024.1080p"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ContentType != testContentTypeMovie {
		t.Errorf("expected movie, got %q", result.ContentType)
	}

	if result.Title != "Test Movie" {
		t.Errorf("expected 'Test Movie', got %q", result.Title)
	}

	if result.Year != 2024 {
		t.Errorf("expected 2024, got %d", result.Year)
	}
}

func TestClassify_EmptyContent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := makeChoiceResp(`{}`)

		w.Header().Set("Content-Type", "application/json")

		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New(Config{Name: "test", BaseURL: srv.URL, Model: "test"})

	_, err := p.Classify(context.Background(), llm.ClassifyInput{Name: "Test"})
	if err == nil {
		t.Fatal("expected error for empty content_type")
	}
}

func TestClassify_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := makeChoiceResp("not json")

		w.Header().Set("Content-Type", "application/json")

		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New(Config{Name: "test", BaseURL: srv.URL, Model: "test"})

	_, err := p.Classify(context.Background(), llm.ClassifyInput{Name: "Test"})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestClassify_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": {"message": "unauthorized", "type": "auth_error"}}`))
	}))
	defer srv.Close()

	p := New(Config{Name: "test", BaseURL: srv.URL, Model: "test"})

	_, err := p.Classify(context.Background(), llm.ClassifyInput{Name: "Test"})
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestClassify_ServerErrorRetry(t *testing.T) {
	t.Parallel()

	attempts := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++

		if attempts <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp := makeChoiceResp(fmt.Sprintf(`{"content_type": %q, "title": "After Retry"}`, testContentTypeMovie))

		w.Header().Set("Content-Type", "application/json")

		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New(Config{Name: "test", BaseURL: srv.URL, Model: "test"})

	result, err := p.Classify(context.Background(), llm.ClassifyInput{Name: "Test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "After Retry" {
		t.Errorf("expected 'After Retry', got %q", result.Title)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestClassify_Timeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := New(Config{
		Name:    "test",
		BaseURL: srv.URL,
		Model:   "test",
		Timeout: 50 * time.Millisecond,
	})

	_, err := p.Classify(context.Background(), llm.ClassifyInput{Name: "Test"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestDefaultTimeout(t *testing.T) {
	t.Parallel()

	cfg := Config{Name: "test"}

	if cfg.timeout() != defaultTimeout {
		t.Errorf("expected %v, got %v", defaultTimeout, cfg.timeout())
	}
}

func TestBuildRequest_IncludesAuth(t *testing.T) {
	t.Parallel()

	p := New(Config{Name: "test", BaseURL: "http://localhost:8080", Model: "m", APIKey: "sk-test"})
	c := p.(*client)

	data, err := c.buildRequest(llm.ClassifyInput{Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	var req chatRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}

	if req.Model != "m" {
		t.Errorf("expected model m, got %q", req.Model)
	}

	if len(req.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(req.Messages))
	}

	if req.Messages[0].Role != "system" {
		t.Errorf("expected system role, got %q", req.Messages[0].Role)
	}

	if req.Messages[1].Role != "user" {
		t.Errorf("expected user role, got %q", req.Messages[1].Role)
	}
}

func TestUserMessage_IncludesFiles(t *testing.T) {
	t.Parallel()

	p := New(Config{Name: "test", BaseURL: "http://localhost:8080", Model: "m"})
	c := p.(*client)
	input := llm.ClassifyInput{
		Name:  "Test.Name.2024",
		Files: []string{"file1.mkv", "folder/file2.nfo"},
	}

	msg := c.buildUserMessage(input)
	if !strings.Contains(msg, "file1.mkv") {
		t.Errorf("expected file1.mkv in message")
	}

	if !strings.Contains(msg, "folder/file2.nfo") {
		t.Errorf("expected folder/file2.nfo in message")
	}
}

func TestUserMessage_FileLimit(t *testing.T) {
	t.Parallel()

	p := New(Config{Name: "test"})
	c := p.(*client)

	files := make([]string, 0, 50)

	for i := range 50 {
		files = append(files, fmt.Sprintf("file%d.mkv", i))
	}

	input := llm.ClassifyInput{Name: "Test", Files: files}

	msg := c.buildUserMessage(input)
	if strings.Count(msg, "File: ") > 20 {
		t.Errorf("too many files in message: got %d, want ≤20", strings.Count(msg, "File: "))
	}
}

func TestSystemPrompt_Default(t *testing.T) {
	t.Parallel()

	p := New(Config{Name: "test"})
	c := p.(*client)

	msg := c.buildSystemMessage(llm.ClassifyInput{ContentTypes: "movie, tv_show"})
	if !strings.Contains(msg, "movie, tv_show") {
		t.Errorf("expected content types in system prompt")
	}

	if !strings.Contains(msg, "valid JSON") && !strings.Contains(msg, "valid json") {
		t.Errorf("expected JSON instruction")
	}
}

func TestSystemPrompt_Custom(t *testing.T) {
	t.Parallel()

	p := New(Config{
		Name:         "test",
		SystemPrompt: "Custom system {{.ContentTypes}}",
	})
	c := p.(*client)

	msg := c.buildSystemMessage(llm.ClassifyInput{ContentTypes: testContentTypeMovie})
	if msg != "Custom system movie" {
		t.Errorf("expected 'Custom system movie', got %q", msg)
	}
}

func TestDefaultConfigTimeout(t *testing.T) {
	t.Parallel()

	cfg := Config{Name: "test"}

	if cfg.timeout() != 30*time.Second {
		t.Errorf("expected 30s, got %v", cfg.timeout())
	}
}

func TestCustomConfigTimeout(t *testing.T) {
	t.Parallel()

	cfg := Config{Name: "test", Timeout: 5 * time.Second}

	if cfg.timeout() != 5*time.Second {
		t.Errorf("expected 5s, got %v", cfg.timeout())
	}
}
