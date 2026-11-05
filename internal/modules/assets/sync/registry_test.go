package sync

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

type fixtureProvider struct {
	name  string
	plan  func(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error)
	fetch func(context.Context, Unit, *credentials.CloudKey) (Snapshot, error)
}

func (p *fixtureProvider) Name() string { return p.name }
func (p *fixtureProvider) Plan(ctx context.Context, row models.CloudAccount, key *credentials.CloudKey) (Plan, error) {
	return p.plan(ctx, row, key)
}
func (p *fixtureProvider) Fetch(ctx context.Context, unit Unit, key *credentials.CloudKey) (Snapshot, error) {
	return p.fetch(ctx, unit, key)
}

func TestRegistryRejectsDuplicatesUnsupportedAndNilProviders(t *testing.T) {
	aws := &fixtureProvider{name: "aws"}
	registry, err := NewRegistry(aws)
	require.NoError(t, err)
	got, err := registry.Lookup("aws")
	require.NoError(t, err)
	require.Same(t, aws, got)
	for _, name := range []string{"aliyun", "gcp", "AWS", "", "other"} {
		got, err := registry.Lookup(name)
		require.Error(t, err)
		require.Nil(t, got)
	}
	_, err = NewRegistry(aws, aws)
	require.Error(t, err)
	_, err = NewRegistry(&fixtureProvider{name: "other"})
	require.Error(t, err)
	_, err = NewRegistry(nil)
	require.Error(t, err)
	var typedNil *fixtureProvider
	_, err = NewRegistry(typedNil)
	require.Error(t, err)
}

func TestProviderPlanValidationRejectsUnsafeUnitsAndBudgetOverflow(t *testing.T) {
	row := models.CloudAccount{ID: 7, Provider: "gcp", ProjectID: "target", CloudKeyID: 9, Enabled: true, ScopeRevision: 1, EnabledRegions: models.StringArray{"us-central1"}}
	valid := Unit{Key: "instances/us-central1-a", ResourceType: "instance", LocationType: "zone", Location: "us-central1-a", Region: "us-central1", Zone: "us-central1-a", Budget: time.Second}
	require.NoError(t, validatePlan(row, Plan{Units: []Unit{valid}}))
	for _, change := range []func(*Unit){
		func(u *Unit) { u.Key = "" },
		func(u *Unit) { u.Key = "secret\nkey" },
		func(u *Unit) { u.ResourceType = "planning" },
		func(u *Unit) { u.ResourceType = "bucket" },
		func(u *Unit) { u.Location = "us-central1-b" },
		func(u *Unit) { u.Region = "other-region" },
		func(u *Unit) { u.Zone = "other-a"; u.Location = u.Zone },
		func(u *Unit) { u.Budget = 0 },
		func(u *Unit) { u.Budget = -time.Second },
		func(u *Unit) { u.Budget = time.Duration(math.MaxInt64) },
	} {
		bad := valid
		change(&bad)
		require.Error(t, validatePlan(row, Plan{Units: []Unit{bad}}), "%+v", bad)
	}
	require.Error(t, validatePlan(row, Plan{Units: []Unit{valid, valid}}))
	require.Error(t, validatePlan(row, Plan{}))
	row.EnabledRegions = nil
	require.Error(t, validatePlan(row, Plan{Units: []Unit{{Key: "network/global", ResourceType: "network", LocationType: "global", Location: "global", Budget: time.Second}}}), "GCP still requires an explicit regional target")
	require.Equal(t, 3*time.Second, boundedPlanBudget(Plan{Units: []Unit{{Budget: time.Second}, {Budget: 2 * time.Second}}}, time.Minute))
	require.Equal(t, time.Minute, boundedPlanBudget(Plan{Units: []Unit{{Budget: time.Duration(math.MaxInt64)}, {Budget: time.Second}}}, time.Minute))
}

func TestConfiguredAccountCeilingAndOverflowSafePlanBudget(t *testing.T) {
	engine := NewProviderEngine(nil, nil, nil, nil, time.Minute, 25*time.Minute)
	require.Equal(t, 25*time.Minute, engine.accountTimeout)
	require.Equal(t, 25*time.Minute, boundedPlanBudget(Plan{Units: []Unit{{Budget: 15 * time.Minute}, {Budget: 15 * time.Minute}}}, engine.accountTimeout))
	require.Equal(t, time.Duration(math.MaxInt64), boundedPlanBudget(Plan{Units: []Unit{{Budget: time.Duration(math.MaxInt64) - 1}, {Budget: 10}}}, time.Duration(math.MaxInt64)))
}
