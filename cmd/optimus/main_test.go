package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitCommand(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		wantCmd  string
		wantArgs []string
	}{
		{"bare binary starts server", nil, "server", nil},
		{"server flags without subcommand", []string{"-config", "c.yaml"}, "server", []string{"-config", "c.yaml"}},
		{"explicit subcommand", []string{"migrate", "-dir", "status"}, "migrate", []string{"-dir", "status"}},
		{"help flag is not a server flag", []string{"-h"}, "-h", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, args := splitCommand(tc.args)
			require.Equal(t, tc.wantCmd, cmd)
			require.Equal(t, tc.wantArgs, args)
		})
	}
}
