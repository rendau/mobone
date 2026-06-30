package mobone

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

type GetModelI interface {
	ListColumnMap() map[string]any
	PKColumnMap() map[string]any
}

type WithGetInterceptorI interface {
	GetInterceptor(qb squirrel.SelectBuilder) squirrel.SelectBuilder
}

func (s *ModelStore) Get(ctx context.Context, m GetModelI) (bool, error) {
	con := s.GetConnection(ctx)

	colMap := m.ListColumnMap()
	colNames := make([]string, 0, len(colMap))
	colFieldPointers := make([]any, 0, len(colMap))
	for colName, fieldPointer := range colMap {
		colNames = append(colNames, colName)
		colFieldPointers = append(colFieldPointers, fieldPointer)
	}

	if len(colNames) == 0 {
		return false, fmt.Errorf("no columns")
	}

	queryBuilder := s.QB.Select(colNames...).
		From(s.TableName).
		Limit(1)

	for k, v := range m.PKColumnMap() {
		queryBuilder = queryBuilder.Where(k+` = ?`, v)
	}

	if qbInterceptor, ok := m.(WithGetInterceptorI); ok && qbInterceptor != nil {
		queryBuilder = qbInterceptor.GetInterceptor(queryBuilder)
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return false, fmt.Errorf("fail to build query: %w", err)
	}

	err = con.QueryRow(ctx, query, args...).Scan(colFieldPointers...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("fail to query: %w", err)
	}

	return true, nil
}
