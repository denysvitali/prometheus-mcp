package server

import (
	"testing"
	"time"
)

func TestParseTimeArg(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "rfc3339",
			input: "2024-01-02T03:04:05Z",
			want:  time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		{
			name:  "rfc3339nano",
			input: "2024-01-02T03:04:05.123456789Z",
			want:  time.Date(2024, 1, 2, 3, 4, 5, 123456789, time.UTC),
		},
		{
			name:  "unix seconds int",
			input: "1704164645",
			want:  time.Unix(1704164645, 0).UTC(),
		},
		{
			name:  "unix seconds float",
			input: "1704164645.5",
			want:  time.Unix(1704164645, int64(0.5*1e9)).UTC(),
		},
		{
			name:    "invalid",
			input:   "not-a-time",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTimeArg(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseTimeArgEmptyUsesNow(t *testing.T) {
	got, err := parseTimeArg("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(got) > time.Second {
		t.Fatalf("expected time close to now, got %v", got)
	}
}
