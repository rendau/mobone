package mobone

import (
	"context"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
)

type connectionGetterI interface {
	GetConnection(ctx context.Context) ConnectionI
}

type ModelStore struct {
	Con                *pgxpool.Pool
	TransactionManager connectionGetterI
	QB                 squirrel.StatementBuilderType
	TableName          string
}

func (s *ModelStore) GetConnection(ctx context.Context) ConnectionI {
	if s.TransactionManager != nil {
		return s.TransactionManager.GetConnection(ctx)
	}

	return s.Con
}
