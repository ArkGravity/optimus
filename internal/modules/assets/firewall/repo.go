package firewall

import (
	"context"

	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/modules/assets/query"
)

type Repo struct{ query *query.Repo }

func NewRepo(db *gorm.DB) *Repo { return &Repo{query: query.NewRepo(db)} }
func (r *Repo) List(ctx context.Context, f query.Filter) ([]Summary, int64, error) {
	rows, total, err := r.query.ListFirewalls(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	items := make([]Summary, len(rows))
	for i, row := range rows {
		items[i] = summarize(row)
	}
	return items, total, nil
}
func (r *Repo) FindByKey(ctx context.Context, key string, includeDeleted bool) (*Detail, error) {
	return r.query.FindFirewall(ctx, key, includeDeleted)
}
