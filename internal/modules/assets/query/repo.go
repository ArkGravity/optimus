// Package query provides provider-neutral, read-only asset projections.
package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/netip"
	"strings"
	"time"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Filter is shared by the family reads. NetworkRef is always the full native reference.
type Filter struct {
	Provider       string
	AccountID      uint64
	ProjectID      string
	Region         string
	Zone           string
	Q              string
	IncludeDeleted bool
	Page           int
	Size           int
	State          string
	Status         string
	Engine         string
	NetworkRef     string
	IP             string
	PrivateIPOnly  bool
	TagKey         string
	TagValue       string
}

func (f Filter) Validate() error {
	if f.Provider != "" && f.Provider != "aws" && f.Provider != "aliyun" && f.Provider != "gcp" {
		return invalid("invalid asset provider")
	}
	if f.AccountID > math.MaxInt64 {
		return invalid("account_id exceeds database range")
	}
	if f.Page < 1 || f.Size < 1 || f.Size > 200 {
		return invalid("page must be positive and size must be between 1 and 200")
	}
	if f.Page-1 > math.MaxInt/f.Size {
		return invalid("pagination offset exceeds supported range")
	}
	if f.TagValue != "" && f.TagKey == "" {
		return invalid("tag_value requires tag_key")
	}
	if f.IP != "" {
		if address, err := netip.ParseAddr(f.IP); err != nil || address.Zone() != "" {
			return invalid("invalid IP address")
		}
	}
	return nil
}
func invalid(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}

type Base struct {
	ID               uint64            `json:"id"`
	AssetKey         string            `json:"asset_key"`
	Provider         string            `json:"provider"`
	CloudAccountID   uint64            `json:"cloud_account_id"`
	CloudAccountName string            `json:"cloud_account_name,omitempty"`
	ProjectID        string            `json:"project_id"`
	LocationType     string            `json:"location_type"`
	Location         string            `json:"location"`
	Region           string            `json:"region"`
	Zone             string            `json:"zone"`
	NativeID         string            `json:"native_id"`
	NativeStatus     string            `json:"native_status"`
	Name             string            `json:"name"`
	Tags             map[string]string `json:"tags"`
	LastSeenAt       time.Time         `json:"last_seen_at"`
	Deleted          bool              `json:"deleted"`
}

type projectionRow struct {
	ID               uint64
	Provider         string
	CloudAccountID   uint64
	CloudAccountName string
	ProjectID        string
	Region           string
	Zone             string
	LocationType     string
	Location         string
	NativeID         string
	NativeStatus     string
	Name             string
	Tags             datatypes.JSON
	LastSeenAt       time.Time
	DeletedAt        *time.Time
	Facts            datatypes.JSON
	NativeDetails    datatypes.JSON
}

func (row projectionRow) base(family string) Base {
	tags := map[string]string{}
	_ = json.Unmarshal(row.Tags, &tags)
	if tags == nil {
		tags = map[string]string{}
	}
	return Base{ID: row.ID, AssetKey: (resource.AssetKey{Provider: row.Provider, Family: family, ID: row.ID}).String(), Provider: row.Provider, CloudAccountID: row.CloudAccountID, CloudAccountName: row.CloudAccountName, ProjectID: row.ProjectID, LocationType: row.LocationType, Location: row.Location, Region: row.Region, Zone: row.Zone, NativeID: row.NativeID, NativeStatus: row.NativeStatus, Name: row.Name, Tags: tags, LastSeenAt: row.LastSeenAt, Deleted: row.DeletedAt != nil}
}

// All arguments here originate in static family definitions, never in request input.
func projection(provider, table, nativeID, name, status, zone, scope, facts, details string) string {
	locationType, location := "'region'", "r.region"
	if scope == "global" {
		locationType, location = "'global'", "'global'"
	}
	if scope == "zone" {
		locationType, location = "'zone'", zone
	}
	return "SELECT r.id, '" + provider + "'::text AS provider, r.cloud_account_id, COALESCE(a.name,'') AS cloud_account_name, COALESCE(a.project_id,'') AS project_id, r.region::text, " + zone + "::text AS zone, " + locationType + "::text AS location_type, " + location + "::text AS location, " + nativeID + "::text AS native_id, " + status + "::text AS native_status, " + name + "::text AS name, r.tags, r.last_seen_at, r.deleted_at, " + facts + " AS facts, " + details + " AS native_details FROM " + table + " r LEFT JOIN cloud_accounts a ON a.id = r.cloud_account_id"
}

func union(sources map[string]string, provider string) string {
	if provider != "" {
		return sources[provider]
	}
	return sources["aws"] + " UNION ALL " + sources["aliyun"] + " UNION ALL " + sources["gcp"]
}

func commonWhere(f Filter) (string, []any) {
	clauses := []string{"TRUE"}
	args := []any{}
	if !f.IncludeDeleted {
		clauses = append(clauses, "deleted_at IS NULL")
	}
	add := func(condition string, value any) { clauses = append(clauses, condition); args = append(args, value) }
	if f.Provider != "" {
		add("provider = ?", f.Provider)
	}
	if f.AccountID != 0 {
		add("cloud_account_id = ?", f.AccountID)
	}
	if f.ProjectID != "" {
		add("project_id = ?", f.ProjectID)
	}
	if f.Region != "" {
		add("region = ?", f.Region)
	}
	if f.Zone != "" {
		add("zone = ?", f.Zone)
	}
	if f.Q != "" {
		term := "%" + f.Q + "%"
		clauses = append(clauses, `(name ILIKE ? OR native_id ILIKE ? OR facts->>'private_ip' ILIKE ? OR facts->>'group_name' ILIKE ? OR facts->>'description' ILIKE ? OR EXISTS (SELECT 1 FROM jsonb_each_text(tags) tag WHERE tag.value ILIKE ?) OR EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(facts->'endpoints','[]'::jsonb)) ep WHERE ep->>'address' ILIKE ?))`)
		args = append(args, term, term, term, term, term, term, term)
	}
	if f.TagKey != "" {
		if f.TagValue == "" {
			add("jsonb_exists(tags, ?)", f.TagKey)
		} else {
			raw, _ := json.Marshal(map[string]string{f.TagKey: f.TagValue})
			add("tags @> ?::jsonb", string(raw))
		}
	}
	return strings.Join(clauses, " AND "), args
}

const stableOrder = "(deleted_at IS NOT NULL) ASC, last_seen_at DESC, provider ASC, id DESC"

func (r *Repo) page(ctx context.Context, sources map[string]string, f Filter, extra string, extraArgs []any) ([]projectionRow, int64, error) {
	if err := f.Validate(); err != nil {
		return nil, 0, err
	}
	relation := union(sources, f.Provider)
	where, args := commonWhere(f)
	if extra != "" {
		where += " AND (" + extra + ")"
		args = append(args, extraArgs...)
	}
	var rows []projectionRow
	var total int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		base := " FROM (" + relation + ") assets WHERE " + where
		if err := tx.Raw("SELECT count(*)"+base, args...).Scan(&total).Error; err != nil {
			return err
		}
		pageArgs := append(append([]any{}, args...), f.Size, (f.Page-1)*f.Size)
		return tx.Raw("SELECT *"+base+" ORDER BY "+stableOrder+" LIMIT ? OFFSET ?", pageArgs...).Scan(&rows).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return rows, total, err
}

func (r *Repo) find(ctx context.Context, sources map[string]string, key, family string, includeDeleted bool) (*projectionRow, error) {
	parsed, err := resource.ParseAssetKey(key, family)
	if err != nil {
		return nil, invalid("invalid asset key")
	}
	rows, _, err := r.page(ctx, sources, Filter{Provider: parsed.Provider, IncludeDeleted: includeDeleted, Page: 1, Size: 1}, "id = ?", []any{parsed.ID})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &rows[0], nil
}
