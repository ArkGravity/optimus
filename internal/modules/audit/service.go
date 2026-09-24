package audit

import (
	"context"
	"encoding/json"

	"github.com/logic3579/optimus/internal/infra/pagination"
	"github.com/logic3579/optimus/internal/models"
)

type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, q ListQuery, p pagination.Params) (pagination.Page[LogEntry], error) {
	rows, total, err := s.repo.List(ctx, q, p)
	if err != nil {
		return pagination.Page[LogEntry]{}, err
	}
	items := make([]LogEntry, 0, len(rows))
	for _, r := range rows {
		items = append(items, toEntry(r))
	}
	return pagination.Of(items, total, p), nil
}

func toEntry(r models.AuditLog) LogEntry {
	return LogEntry{
		ID: r.ID, UserID: r.UserID, Action: r.Action,
		TargetType: r.TargetType, TargetID: r.TargetID,
		Payload: json.RawMessage(r.Payload), IP: r.IP, UserAgent: r.UserAgent,
		CreatedAt: r.CreatedAt,
	}
}
