package model

import (
	"time"

	"github.com/Masterminds/squirrel"

	"github.com/rendau/mobone/v2"
)

type Select struct {
	Id        int
	CreatedAt time.Time
	UpdatedAt time.Time
	Name      string
	Flag      bool
	Contact   Contact
}

func (m *Select) ListColumnMap() map[string]any {
	return map[string]any{
		"id":         &m.Id,
		"created_at": &m.CreatedAt,
		"updated_at": &m.UpdatedAt,
		"name":       &m.Name,
		"flag":       &m.Flag,
		"contact":    &m.Contact,
	}
}

func (m *Select) PKColumnMap() map[string]any {
	return map[string]any{
		"id": m.Id,
	}
}

func (m *Select) DefaultSortColumns() []string {
	return []string{"id"}
}

func (m *Select) ListInterceptor(qb squirrel.SelectBuilder, params mobone.ListParams) squirrel.SelectBuilder {
	if len(params.Columns) == 1 && params.Columns[0] == "id" {
		qb = qb.CrossJoin("(select 7) as s")
	}

	return qb
}
