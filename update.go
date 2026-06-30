package mobone

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type UpdateModelI interface {
	UpdateColumnMap() map[string]any
	PKColumnMap() map[string]any
}

type UpdateCreateModelI interface {
	UpdateModelI
	CreateModelI
}

func (s *ModelStore) Update(ctx context.Context, m UpdateModelI) error {
	query, args, err := s.buildUpdateQuery(m)
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

func (s *ModelStore) buildUpdateQuery(m UpdateModelI) (string, []any, error) {
	queryBuilder := s.QB.Update(s.TableName).
		SetMap(m.UpdateColumnMap())

	for k, v := range m.PKColumnMap() {
		queryBuilder = queryBuilder.Where(k+` = ?`, v)
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return "", nil, err
	}

	return query, args, nil
}

func (s *ModelStore) UpdateMany(ctx context.Context, models []UpdateModelI) (finalError error) {
	if len(models) == 0 {
		return nil
	}

	con := s.GetConnection(ctx)

	batch := &pgx.Batch{}
	for i, m := range models {
		query, args, err := s.buildUpdateQuery(m)
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

func (s *ModelStore) UpdateOrCreate(ctx context.Context, m UpdateCreateModelI) error {
	query, args, err := s.buildUpdateOrCreateQuery(m)
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

func (s *ModelStore) buildUpdateOrCreateQuery(m UpdateCreateModelI) (string, []any, error) {
	pkColumnMap := m.PKColumnMap()
	pkColumnNames := make([]string, 0, len(pkColumnMap))
	for k := range pkColumnMap {
		pkColumnNames = append(pkColumnNames, k)
	}

	updateColumnMap := m.UpdateColumnMap()
	updateColumnNames := make([]string, 0, len(updateColumnMap))
	updateColumnValues := make([]any, 0, len(updateColumnMap))
	for k, v := range updateColumnMap {
		updateColumnNames = append(updateColumnNames, k)
		updateColumnValues = append(updateColumnValues, v)
	}

	queryBuilder := s.QB.Insert(s.TableName+" as t").
		SetMap(m.CreateColumnMap()).
		Suffix(`ON CONFLICT (`+strings.Join(pkColumnNames, ",")+`) DO UPDATE SET `+strings.Join(updateColumnNames, " = ?, ")+` = ?`, updateColumnValues...)

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return "", nil, err
	}

	return query, args, nil
}

func (s *ModelStore) UpdateOrCreateMany(ctx context.Context, models []UpdateCreateModelI) (finalError error) {
	if len(models) == 0 {
		return nil
	}

	con := s.GetConnection(ctx)

	batch := &pgx.Batch{}
	for i, m := range models {
		query, args, err := s.buildUpdateOrCreateQuery(m)
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
