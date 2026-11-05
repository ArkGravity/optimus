package resource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocationValidate(t *testing.T) {
	for _, location := range []Location{
		{Type: "global", Name: "global"},
		{Type: "region", Name: "us-central1", Region: "us-central1"},
		{Type: "zone", Name: "us-central1-a", Region: "us-central1", Zone: "us-central1-a"},
	} {
		t.Run(location.Type, func(t *testing.T) {
			require.NoError(t, location.Validate())
		})
	}
}

func TestLocationValidateRejectsInconsistentScope(t *testing.T) {
	for _, tc := range []struct {
		name     string
		location Location
	}{
		{"empty", Location{}},
		{"unknown type", Location{Type: "project", Name: "global"}},
		{"global name", Location{Type: "global", Name: "us-central1"}},
		{"global region", Location{Type: "global", Name: "global", Region: "us-central1"}},
		{"global zone", Location{Type: "global", Name: "global", Zone: "us-central1-a"}},
		{"region missing", Location{Type: "region"}},
		{"region name missing", Location{Type: "region", Region: "us-central1"}},
		{"region name mismatch", Location{Type: "region", Name: "europe-west1", Region: "us-central1"}},
		{"region with zone", Location{Type: "region", Name: "us-central1", Region: "us-central1", Zone: "us-central1-a"}},
		{"zone missing region", Location{Type: "zone", Name: "us-central1-a", Zone: "us-central1-a"}},
		{"zone missing zone", Location{Type: "zone", Name: "us-central1-a", Region: "us-central1"}},
		{"zone name missing", Location{Type: "zone", Region: "us-central1", Zone: "us-central1-a"}},
		{"zone name mismatch", Location{Type: "zone", Name: "us-central1-b", Region: "us-central1", Zone: "us-central1-a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.location.Validate())
		})
	}
}
