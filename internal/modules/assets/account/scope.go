package account

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var regionPatterns = map[string]*regexp.Regexp{
	"aws":    regexp.MustCompile(`^[a-z]{2}(?:-[a-z]+)+-\d+$`),
	"aliyun": regexp.MustCompile(`^[a-z]{2}(?:-[a-z]+)+(?:-\d+)?$`),
	"gcp":    regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+\d+$`),
}

func configError(message string) error {
	return apperr.New(errs.CodeAssetsConfig, errs.KeyConfig, message)
}

func validateTarget(provider, project string) error {
	if _, ok := regionPatterns[provider]; !ok {
		return apperr.New(errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported, "unsupported cloud provider")
	}
	if (provider == "gcp" && (strings.TrimSpace(project) == "" || project != strings.TrimSpace(project))) || (provider != "gcp" && project != "") {
		return configError("a target project is required only for GCP")
	}
	return nil
}

func normalizeRegions(provider string, regions []string) ([]string, error) {
	pattern, ok := regionPatterns[provider]
	if !ok {
		return nil, apperr.New(errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported, "unsupported cloud provider")
	}
	if len(regions) == 0 {
		return nil, apperr.New(errs.CodeAssetsRegionInvalid, errs.KeyRegionInvalid, "at least one region is required")
	}
	normalized := make([]string, len(regions))
	for i, region := range regions {
		if len(region) > 64 || !pattern.MatchString(region) {
			return nil, apperr.New(errs.CodeAssetsRegionInvalid, errs.KeyRegionInvalid, "invalid provider region")
		}
		normalized[i] = region
	}
	slices.Sort(normalized)
	return slices.Compact(normalized), nil
}

// Scope captures collection eligibility before remote calls. Revision changes
// invalidate in-flight work even if a region was removed and later restored.
type Scope struct {
	AccountID         uint64
	Provider          string
	ProjectID         string
	ExternalAccountID string
	CloudKeyID        uint64
	Revision          int64
	Enabled           bool
}

func CaptureScope(row models.CloudAccount) Scope {
	return Scope{AccountID: row.ID, Provider: row.Provider, ProjectID: row.ProjectID, ExternalAccountID: row.ExternalAccountID, CloudKeyID: row.CloudKeyID, Revision: row.ScopeRevision, Enabled: row.Enabled}
}

// PinIdentityTx must run before publication/pruning in the same transaction.
// verifiedIdentity comes only from the provider's read-only STS identity call.
// The returned row includes the pinned identity for subsequent engine units.
func (r *Repo) PinIdentityTx(ctx context.Context, tx *gorm.DB, captured Scope, verifiedIdentity string) (*models.CloudAccount, error) {
	row, err := r.FindByIDForUpdate(ctx, tx, captured.AccountID)
	if err != nil {
		return nil, err
	}
	if !row.Enabled || !captured.Enabled || row.Provider != captured.Provider || row.ProjectID != captured.ProjectID || row.ScopeRevision != captured.Revision || row.CloudKeyID != captured.CloudKeyID {
		return nil, apperr.New(errs.CodeAssetsSyncBusy, errs.KeySyncBusy, "cloud account scope changed during collection")
	}
	if row.Provider == "gcp" {
		return row, nil
	}
	if row.Provider != "aws" && row.Provider != "aliyun" {
		return nil, configError("unsupported identity provider")
	}
	if strings.TrimSpace(verifiedIdentity) == "" || verifiedIdentity != strings.TrimSpace(verifiedIdentity) {
		return nil, configError("provider identity is empty or invalid")
	}
	if row.ExternalAccountID != "" && row.ExternalAccountID != verifiedIdentity {
		return nil, apperr.New(errs.CodeAssetsIdentityMismatch, errs.KeyIdentityMismatch, "verified provider identity does not match this cloud account")
	}
	if row.ExternalAccountID == "" {
		if _, err := r.UpdateTx(ctx, tx, row.ID, map[string]any{"external_account_id": verifiedIdentity}); err != nil {
			return nil, err
		}
		row.ExternalAccountID = verifiedIdentity
	}
	return row, nil
}

// ErrScopeIneligible rejects work whose captured account or location is no longer eligible.
var ErrScopeIneligible = errors.New("asset collection scope is no longer eligible")

// LockScopeTx locks and rechecks the captured binding before any resource writes.
// The caller must supply an active transaction and roll it back on any error.
// Global GCP units remain eligible independently of enabled-region changes;
// their captured revision must still match the current account revision.
func LockScopeTx(ctx context.Context, tx *gorm.DB, captured Scope, location resource.Location) (*models.CloudAccount, error) {
	if tx == nil || tx.Statement == nil {
		return nil, gorm.ErrInvalidTransaction
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, gorm.ErrInvalidTransaction
	}
	if location.Validate() != nil || captured.AccountID == 0 || captured.CloudKeyID == 0 || captured.Revision < 1 || !captured.Enabled {
		return nil, ErrScopeIneligible
	}
	// Planning owns authoritative zone discovery; this gate also verifies the
	// GCP zone hierarchy without making network requests while holding a lock.
	if captured.Provider == "gcp" && location.Type == "zone" {
		suffix, matches := strings.CutPrefix(location.Zone, location.Region+"-")
		if !matches || suffix == "" {
			return nil, ErrScopeIneligible
		}
	}
	var row models.CloudAccount
	if err := tx.Session(&gorm.Session{NewDB: true, Context: ctx}).Clauses(clause.Locking{Strength: "UPDATE"}).Where("cloud_accounts.deleted_at IS NULL").First(&row, captured.AccountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScopeIneligible
		}
		return nil, err
	}
	if !row.Enabled || row.Provider != captured.Provider || row.ProjectID != captured.ProjectID || row.CloudKeyID != captured.CloudKeyID || row.ExternalAccountID != captured.ExternalAccountID || row.ScopeRevision != captured.Revision {
		return nil, ErrScopeIneligible
	}
	if location.Type == "global" {
		if row.Provider != "gcp" {
			return nil, ErrScopeIneligible
		}
	} else if !slices.Contains([]string(row.EnabledRegions), location.Region) {
		return nil, ErrScopeIneligible
	}
	return &row, nil
}
