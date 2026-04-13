package mobone

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ListModelI interface {
	ListColumnMap() map[string]any
	DefaultSortColumns() []string
}

type GetModelI interface {
	ListColumnMap() map[string]any
	PKColumnMap() map[string]any
}

type CreateModelI interface {
	CreateColumnMap() map[string]any
	ReturningColumnMap() map[string]any
}

type UpdateModelI interface {
	UpdateColumnMap() map[string]any
	PKColumnMap() map[string]any
}

type UpdateCreateModelI interface {
	UpdateModelI
	CreateModelI
}

type DeleteModelI interface {
	PKColumnMap() map[string]any
}

type WithListInterceptorI interface {
	ListInterceptor(qb squirrel.SelectBuilder, params ListParams) squirrel.SelectBuilder
}

type WithGetInterceptorI interface {
	GetInterceptor(qb squirrel.SelectBuilder) squirrel.SelectBuilder
}

type connectionGetterI interface {
	GetConnection(ctx context.Context) ConnectionI
}

type ListParams struct {
	Conditions           map[string]any
	ConditionExpressions map[string][]any
	Distinct             bool
	Columns              []string
	Page                 int64
	PageSize             int64
	WithTotalCount       bool
	OnlyCount            bool
	Sort                 []string
	CustomConditions     map[string]string
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

func (s *ModelStore) Create(ctx context.Context, m CreateModelI) error {
	con := s.GetConnection(ctx)

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
		return fmt.Errorf("fail to build query: %w", err)
	}

	if len(returningColumnNames) > 0 {
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

func (s *ModelStore) Update(ctx context.Context, m UpdateModelI) error {
	con := s.GetConnection(ctx)

	queryBuilder := s.QB.Update(s.TableName).
		SetMap(m.UpdateColumnMap())

	for k, v := range m.PKColumnMap() {
		queryBuilder = queryBuilder.Where(k+` = ?`, v)
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return fmt.Errorf("fail to build query: %w", err)
	}

	// fmt.Println(query, args)

	_, err = con.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("fail to exec: %w", err)
	}

	return nil
}

func (s *ModelStore) UpdateOrCreate(ctx context.Context, m UpdateCreateModelI) error {
	con := s.GetConnection(ctx)

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
		return fmt.Errorf("fail to build query: %w", err)
	}

	_, err = con.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("fail to exec: %w", err)
	}

	return nil
}

func (s *ModelStore) CreateIfNotExist(ctx context.Context, m UpdateCreateModelI) error {
	con := s.GetConnection(ctx)

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
		return fmt.Errorf("fail to build query: %w", err)
	}

	_, err = con.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("fail to exec: %w", err)
	}

	return nil
}

func (s *ModelStore) Delete(ctx context.Context, m DeleteModelI) error {
	con := s.GetConnection(ctx)

	queryBuilder := s.QB.Delete(s.TableName)

	for k, v := range m.PKColumnMap() {
		queryBuilder = queryBuilder.Where(k+` = ?`, v)
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return fmt.Errorf("fail to build query: %w", err)
	}

	_, err = con.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("fail to exec: %w", err)
	}

	return nil
}

func (s *ModelStore) List(ctx context.Context, params ListParams, itemConstructor func(add bool) ListModelI) (int64, error) {
	con := s.GetConnection(ctx)

	queryBuilder := s.QB.Select().From(s.TableName)

	// conditions
	if params.Conditions != nil {
		queryBuilder = queryBuilder.Where(params.Conditions)
	}
	if params.ConditionExpressions != nil {
		for expression, args := range params.ConditionExpressions {
			queryBuilder = queryBuilder.Where(expression, args...)
		}
	}

	var totalCount int64

	listItemInstance := itemConstructor(false)

	// construct column names
	allowedColMap := listItemInstance.ListColumnMap()
	colNames := make([]string, 0, len(params.Columns))
	if len(params.Columns) > 0 {
		var ok bool
		for _, colName := range params.Columns {
			if _, ok = allowedColMap[colName]; ok {
				colNames = append(colNames, colName)
			}
		}
	} else {
		for colName := range allowedColMap {
			colNames = append(colNames, colName)
		}
	}
	if len(colNames) == 0 {
		return 0, fmt.Errorf("no columns")
	}

	if qbInterceptor, ok := listItemInstance.(WithListInterceptorI); ok && qbInterceptor != nil {
		queryBuilder = qbInterceptor.ListInterceptor(queryBuilder, params)
	}

	// total count
	if params.WithTotalCount || params.OnlyCount {
		if params.Distinct {
			queryBuilder = queryBuilder.Column(`count(distinct (` + strings.Join(colNames, ",") + `))`)
		} else {
			queryBuilder = queryBuilder.Column(`count(*)`)
		}

		query, args, err := queryBuilder.ToSql()
		if err != nil {
			return 0, fmt.Errorf("fail to build query: %w", err)
		}

		err = con.QueryRow(ctx, query, args...).Scan(&totalCount)
		if err != nil {
			return 0, fmt.Errorf("fail to query: %w", err)
		}

		if params.OnlyCount {
			return totalCount, nil
		}

		queryBuilder = queryBuilder.RemoveColumns()
	}

	// apply columns
	if params.Distinct {
		queryBuilder = queryBuilder.Distinct()
	}
	queryBuilder = queryBuilder.Columns(colNames...)

	// pagination
	if params.PageSize > 0 {
		queryBuilder = queryBuilder.Offset(uint64(params.Page * params.PageSize)).Limit(uint64(params.PageSize))
	}

	// sort
	if params.Sort == nil {
		sortColumns := listItemInstance.DefaultSortColumns()
		if len(sortColumns) > 0 {
			queryBuilder = queryBuilder.OrderBy(sortColumns...)
		}
	} else if len(params.Sort) > 0 {
		queryBuilder = queryBuilder.OrderBy(params.Sort...)
	}

	// build query
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return 0, fmt.Errorf("fail to build query: %w", err)
	}

	// slog.Info("List query", "query", query, "args", args)

	// execute query
	rows, err := con.Query(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("fail to query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		m := itemConstructor(true)

		err = rows.Scan(fieldPointersForColNames(m, colNames)...)
		if err != nil {
			return 0, fmt.Errorf("fail to scan: %w", err)
		}
	}
	if err = rows.Err(); err != nil {
		return 0, fmt.Errorf("rows.Err: %w", err)
	}

	return totalCount, nil
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

func fieldPointersForColNames(m ListModelI, colNames []string) []any {
	colMap := m.ListColumnMap()
	result := make([]any, 0, len(colNames))
	for _, colName := range colNames {
		result = append(result, colMap[colName])
	}
	return result
}
