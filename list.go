package mobone

import (
	"context"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
)

type ListModelI interface {
	ListColumnMap() map[string]any
	DefaultSortColumns() []string
}

type WithListInterceptorI interface {
	ListInterceptor(qb squirrel.SelectBuilder, params ListParams) squirrel.SelectBuilder
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

func fieldPointersForColNames(m ListModelI, colNames []string) []any {
	colMap := m.ListColumnMap()
	result := make([]any, 0, len(colNames))
	for _, colName := range colNames {
		result = append(result, colMap[colName])
	}
	return result
}
