package server

import (
	"testing"
	"time"
)

func TestCompletedAtForStatus(t *testing.T) {
	past := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		status      string
		existing    *time.Time
		wantNil     bool
		wantExact   *time.Time // non-nil means result must equal this
		wantRecent  bool       // true means result must be a fresh "now" timestamp
	}{
		// Completed statuses with no existing timestamp → fresh timestamp set.
		{name: "done/nil→set", status: "done", existing: nil, wantRecent: true},
		{name: "completed/nil→set", status: "completed", existing: nil, wantRecent: true},

		// Completed statuses with existing timestamp → preserved, not reset.
		{name: "done/existing→preserve", status: "done", existing: &past, wantExact: &past},
		{name: "completed/existing→preserve", status: "completed", existing: &past, wantExact: &past},

		// Non-completed task statuses → always nil.
		{name: "todo/nil→nil", status: "todo", existing: nil, wantNil: true},
		{name: "in_progress/nil→nil", status: "in_progress", existing: nil, wantNil: true},

		// Non-completed project statuses → always nil.
		{name: "active/nil→nil", status: "active", existing: nil, wantNil: true},
		{name: "backlog/nil→nil", status: "backlog", existing: nil, wantNil: true},
		{name: "blocked/nil→nil", status: "blocked", existing: nil, wantNil: true},
		{name: "abandoned/nil→nil", status: "abandoned", existing: nil, wantNil: true},

		// Unspecified/empty → nil.
		{name: "empty/nil→nil", status: "", existing: nil, wantNil: true},

		// Moving away from completed clears regardless of existing.
		{name: "todo/existing→nil", status: "todo", existing: &past, wantNil: true},
		{name: "in_progress/existing→nil", status: "in_progress", existing: &past, wantNil: true},
		{name: "active/existing→nil", status: "active", existing: &past, wantNil: true},
		{name: "backlog/existing→nil", status: "backlog", existing: &past, wantNil: true},
		{name: "blocked/existing→nil", status: "blocked", existing: &past, wantNil: true},
		{name: "abandoned/existing→nil", status: "abandoned", existing: &past, wantNil: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := time.Now()
			got := completedAtForStatus(tc.status, tc.existing)
			after := time.Now()

			switch {
			case tc.wantNil:
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
			case tc.wantExact != nil:
				if got == nil {
					t.Fatal("got nil, want existing timestamp")
				}
				if !got.Equal(*tc.wantExact) {
					t.Fatalf("got %v, want %v", got, tc.wantExact)
				}
			case tc.wantRecent:
				if got == nil {
					t.Fatal("got nil, want a fresh timestamp")
				}
				if got.Before(before) || got.After(after) {
					t.Fatalf("timestamp %v outside expected range [%v, %v]", got, before, after)
				}
			}
		})
	}
}
