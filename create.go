package mobone

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type CreateModelI interface {
	CreateColumnMap() map[string]any
	ReturningColumnMap() map[string]any
}

func (s *ModelStore) Create(ctx context.Context, m CreateModelI) error {
	query, args, returningFieldPointers, err := s.buildCreateQuery(m)
	if err != nil {
		return fmt.Errorf("fail to build query: %w", err)
	}

	con := s.GetConnection(ctx)

	if len(returningFieldPointers) > 0 {
		err = con.QueryRow(ctx, query, args...).Scan(returningFieldPointers...)
		if err != nil {
			return fmt.Errorf("fail to query: %w", err)
		}
	} else {
		_, err = con.Exec(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("fail to exec: %w", err)
		}
	}

	return nil
}

func (s *ModelStore) buildCreateQuery(m CreateModelI) (string, []any, []any, error) {
	queryBuilder := s.QB.Insert(s.TableName).
		SetMap(m.CreateColumnMap())

	returningColumnMap := m.ReturningColumnMap()
	returningColumnNames := make([]string, 0, len(returningColumnMap))
	returningFieldPointers := make([]any, 0, len(returningColumnMap))
	for k, v := range returningColumnMap {
		returningColumnNames = append(returningColumnNames, k)
		returningFieldPointers = append(returningFieldPointers, v)
	}

	if len(returningColumnNames) > 0 {
		queryBuilder = queryBuilder.Suffix(`RETURNING ` + strings.Join(returningColumnNames, ","))
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return "", nil, nil, err
	}

	return query, args, returningFieldPointers, nil
}

func (s *ModelStore) CreateMany(ctx context.Context, models []CreateModelI) (finalError error) {
	if len(models) == 0 {
		return nil
	}

	con := s.GetConnection(ctx)

	batch := &pgx.Batch{}
	returningFieldPointersByItem := make([][]any, len(models))
	for i, m := range models {
		query, args, returningFieldPointers, err := s.buildCreateQuery(m)
		if err != nil {
			return fmt.Errorf("fail to build query (item %d): %w", i, err)
		}

		batch.Queue(query, args...)
		returningFieldPointersByItem[i] = returningFieldPointers
	}

	results := con.SendBatch(ctx, batch)
	defer func() {
		if err := results.Close(); err != nil && finalError == nil {
			finalError = fmt.Errorf("fail to close batch results: %w", err)
		}
	}()

	for i := range models {
		returningFieldPointers := returningFieldPointersByItem[i]
		if len(returningFieldPointers) > 0 {
			if err := results.QueryRow().Scan(returningFieldPointers...); err != nil {
				return fmt.Errorf("fail to query (item %d): %w", i, err)
			}
			continue
		}

		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("fail to exec (item %d): %w", i, err)
		}
	}

	return nil
}

func (s *ModelStore) CreateIfNotExist(ctx context.Context, m UpdateCreateModelI) error {
	query, args, err := s.buildCreateIfNotExistQuery(m)
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

func (s *ModelStore) buildCreateIfNotExistQuery(m UpdateCreateModelI) (string, []any, error) {
	pkColumnMap := m.PKColumnMap()
	pkColumnNames := make([]string, 0, len(pkColumnMap))
	for k := range pkColumnMap {
		pkColumnNames = append(pkColumnNames, k)
	}

	insertColumnMap := m.CreateColumnMap()
	for k, v := range pkColumnMap {
		insertColumnMap[k] = v
	}

	queryBuilder := s.QB.Insert(s.TableName).
		SetMap(insertColumnMap).
		Suffix(`ON CONFLICT (` + strings.Join(pkColumnNames, ",") + `) DO NOTHING`)

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return "", nil, err
	}

	return query, args, nil
}

func (s *ModelStore) CreateIfNotExistMany(ctx context.Context, models []UpdateCreateModelI) (finalError error) {
	if len(models) == 0 {
		return nil
	}

	con := s.GetConnection(ctx)

	batch := &pgx.Batch{}
	for i, m := range models {
		query, args, err := s.buildCreateIfNotExistQuery(m)
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
