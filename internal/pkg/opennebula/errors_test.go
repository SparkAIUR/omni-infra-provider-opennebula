package opennebula

import (
	"errors"
	"testing"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want ErrorClass
	}{
		{name: "success", err: nil, want: ErrorClassSuccess},
		{name: "not-found", err: ErrNotFound, want: ErrorClassNotFound},
		{name: "auth", err: ErrAuth, want: ErrorClassAuth},
		{name: "quota", err: ErrQuota, want: ErrorClassQuota},
		{name: "policy", err: ErrPolicy, want: ErrorClassPolicy},
		{name: "retryable", err: ErrRetryable, want: ErrorClassRetryable},
		{name: "terminal", err: ErrTerminal, want: ErrorClassTerminal},
		{name: "heuristic-timeout", err: errors.New("request timeout from RPC2"), want: ErrorClassRetryable},
		{name: "heuristic-unknown", err: errors.New("something odd happened"), want: ErrorClassUnknown},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ClassifyError(tt.err); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
