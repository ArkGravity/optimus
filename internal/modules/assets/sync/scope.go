package sync

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"time"

	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

const DefaultAccountTimeout = 15 * time.Minute
const defaultRequestTimeout = 30 * time.Second

var unitKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,255}$`)

var ErrIncompleteSnapshot = errors.New("incomplete asset snapshot")

func PublishSnapshot(ctx context.Context, db *gorm.DB, unit Unit, snapshot Snapshot) (Stats, error) {
	if !snapshot.Complete || snapshot.Publish == nil {
		return Stats{}, ErrIncompleteSnapshot
	}
	var stats Stats
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := account.LockScopeTx(ctx, tx, unit.Scope, unitLocation(unit)); err != nil {
			return err
		}
		var err error
		stats, err = snapshot.Publish(ctx, tx)
		return err
	})
	if err != nil {
		return Stats{}, err
	}
	return stats, nil
}

func unitLocation(unit Unit) resource.Location {
	return resource.Location{Type: unit.LocationType, Name: unit.Location, Region: unit.Region, Zone: unit.Zone}
}

func validatePlan(row models.CloudAccount, plan Plan) error {
	if len(plan.Units) == 0 || len(row.EnabledRegions) == 0 {
		return planConfigError()
	}
	keys := make(map[string]bool, len(plan.Units))
	scopes := make(map[string]bool, len(plan.Units))
	for _, unit := range plan.Units {
		location := unitLocation(unit)
		if !unitKeyPattern.MatchString(unit.Key) || keys[unit.Key] || location.Validate() != nil || unit.Budget <= 0 || unit.Budget > DefaultAccountTimeout {
			return planConfigError()
		}
		keys[unit.Key] = true
		switch unit.ResourceType {
		case "instance", "network", "database", "security_group", "subnet", "firewall":
		default:
			return planConfigError()
		}
		if row.Provider != "gcp" && location.Type != "region" {
			return planConfigError()
		}
		if location.Type != "global" && !slices.Contains([]string(row.EnabledRegions), unit.Region) {
			return planConfigError()
		}
		if row.Provider == "gcp" {
			switch unit.ResourceType {
			case "network", "firewall":
				if location.Type != "global" {
					return planConfigError()
				}
			case "subnet", "database":
				if location.Type != "region" {
					return planConfigError()
				}
			case "instance":
				if location.Type != "zone" || len(unit.Zone) <= len(unit.Region)+1 || unit.Zone[:len(unit.Region)+1] != unit.Region+"-" {
					return planConfigError()
				}
			default:
				return planConfigError()
			}
		}
		scopeKey := unit.ResourceType + "/" + unit.LocationType + "/" + unit.Location
		if scopes[scopeKey] {
			return planConfigError()
		}
		scopes[scopeKey] = true
	}
	return nil
}

func boundedPlanBudget(plan Plan, ceiling time.Duration) time.Duration {
	var total time.Duration
	for _, unit := range plan.Units {
		if unit.Budget >= ceiling-total {
			return ceiling
		}
		total += unit.Budget
	}
	return total
}

func planConfigError() *apperr.BizError {
	return apperr.New(errs.CodeAssetsConfig, errs.KeyConfig, "invalid asset collection configuration")
}
