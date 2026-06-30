package mobone

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type DeleteModelI interface {
	PKColumnMap() map[string]any
}

func (s *ModelStore) Delete(ctx context.Context, m DeleteModelI) error {
	query, args, err := s.buildDeleteQuery(m)
	if err != nil {
		return fmt.Errorf("fail to build query: %w", err)
	}

	con := s.GetConnection(ctx)

	_, err = con.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("fail to exec: %w", err)
	}

	return nil
}

func (s *ModelStore) buildDeleteQuery(m DeleteModelI) (string, []any, error) {
	queryBuilder := s.QB.Delete(s.TableName)

	for k, v := range m.PKColumnMap() {
		queryBuilder = queryBuilder.Where(k+` = ?`, v)
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return "", nil, err
	}

	return query, args, nil
}

func (s *ModelStore) DeleteMany(ctx context.Context, models []DeleteModelI) (finalError error) {
	if len(models) == 0 {
		return nil
	}

	con := s.GetConnection(ctx)

	batch := &pgx.Batch{}
	for i, m := range models {
		query, args, err := s.buildDeleteQuery(m)
		if err != nil {
			return fmt.Errorf("fail to build query (item %d): %w", i, err)
		}

		batch.Queue(query, args...)
	}

	results := con.SendBatch(ctx, batch)
	defer func() {
		if err := results.Close(); err != nil && finalError == nil {
			finalError = fmt.Errorf("fail to close batch results: %w", err)
		}
	}()

	for i := range models {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("fail to exec (item %d): %w", i, err)
		}
	}

	return nil
}
