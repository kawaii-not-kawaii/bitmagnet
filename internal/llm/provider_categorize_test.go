package llm

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestCategorize(t *testing.T) {
	t.Parallel()

	// A concrete net.Error to exercise the errors.As fallback.
	dnsErr := &net.DNSError{Err: "no such host", Name: "lemond.invalid"}

	tests := []struct {
		name string
		err  error
		want ErrorCategory
	}{
		{"nil", nil, ""},
		{"rate limited", fmt.Errorf("openai: %w: 429", ErrRateLimited), CategoryRateLimited},
		{
			"rate limited after retries",
			fmt.Errorf("openai: %w (after 1 retries)", fmt.Errorf("%w: x", ErrRateLimited)),
			CategoryRateLimited,
		},
		{"bad status", fmt.Errorf("openai: %w: 404 not found", ErrBadStatus), CategoryBadStatus},
		{
			"invalid json",
			fmt.Errorf("%w: %w", ErrInvalidJSON, errors.New("unexpected token")),
			CategoryInvalidJSON,
		},
		{"empty content", ErrNoResult, CategoryEmptyContent},
		{"connection", fmt.Errorf("openai: request failed: %w", dnsErr), CategoryConnection},
		{"other", errors.New("something unexpected"), CategoryOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Categorize(tt.err); got != tt.want {
				t.Fatalf("Categorize(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
