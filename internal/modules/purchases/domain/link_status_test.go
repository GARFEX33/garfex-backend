package domain

import "testing"

func TestLinkStatusValid(t *testing.T) {
	tests := []struct {
		status LinkStatus
		want   bool
	}{
		{LinkPending, true},
		{LinkLinked, true},
		{LinkSuspended, true},
		{LinkNotApplicable, true},
		{LinkConflict, true},
		{LinkStatus("BOGUS"), false},
		{LinkStatus(""), false},
	}
	for _, tt := range tests {
		if got := tt.status.Valid(); got != tt.want {
			t.Errorf("LinkStatus(%q).Valid() = %t, want %t", tt.status, got, tt.want)
		}
	}
}
