package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDatabaseClosesOnlyAfterAssetsDrain(t *testing.T) {
	for _, tc := range []struct {
		name      string
		assetsErr error
		wantClose bool
	}{
		{"drained", nil, true},
		{"timed out", context.DeadlineExceeded, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			closed := false
			got := closeDBIfDrained(tc.assetsErr, func() error { closed = true; return nil })
			require.Equal(t, tc.wantClose, got)
			require.Equal(t, tc.wantClose, closed)
		})
	}
	require.False(t, closeDBIfDrained(nil, nil))
}
