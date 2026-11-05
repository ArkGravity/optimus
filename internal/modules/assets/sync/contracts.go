package sync

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

type Scope = account.Scope

// Unit describes one authoritative provider-native collection boundary.
type Unit struct {
	Key, ResourceType, LocationType, Location, Region, Zone string
	Scope                                                   Scope
	Budget                                                  time.Duration
}

type Plan struct {
	Units             []Unit
	ExternalAccountID string
}

type Stats struct{ Seen, SoftDeleted int64 }

// ErrIdentityVerification marks a failed fresh-key identity check. The engine
// stops the account rather than trying additional units with unverified keys.
var ErrIdentityVerification = errors.New("provider identity verification failed")

// Snapshot is constructed by internal provider adapters after all pages succeed.
// Publish captures typed native rows, never SDK clients or response objects.
type Snapshot struct {
	Complete bool
	Publish  func(context.Context, *gorm.DB) (Stats, error)
}

type Provider interface {
	Name() string
	Plan(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error)
	Fetch(context.Context, Unit, *credentials.CloudKey) (Snapshot, error)
}
