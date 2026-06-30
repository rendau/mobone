# mobone — справочник API

`github.com/mechta-market/mobone/v2`. Здесь — точные сигнатуры, поведение генерации SQL и
особенности каждой операции (по исходникам).

## ModelStore

```go
type ModelStore struct {
    Con                *pgxpool.Pool                 // обязателен
    TransactionManager connectionGetterI             // опционально (TransactionManager)
    QB                 squirrel.StatementBuilderType // squirrel-билдер с Dollar
    TableName          string                        // имя таблицы
}

func (s *ModelStore) GetConnection(ctx context.Context) ConnectionI
```

`GetConnection`: при наличии `TransactionManager` делегирует ему (вернёт tx из ctx либо пул),
иначе — `Con`.

## ConnectionI / TransactionManagerI

```go
type ConnectionI interface {
    Exec(ctx, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx, sql string, args ...any) pgx.Row
    SendBatch(ctx, b *pgx.Batch) pgx.BatchResults
}

type TransactionManagerI interface {
    GetConnection(ctx) ConnectionI
    TxFn(ctx, func(context.Context) error) error
}
```

Как `*pgxpool.Pool`, так и `pgx.Tx` удовлетворяют `ConnectionI`.

---

## Get

```go
func (s *ModelStore) Get(ctx, m GetModelI) (bool, error)
```

SQL: `SELECT <ключи ListColumnMap> FROM <table> WHERE <pk> = $n ... LIMIT 1`.
- Колонки и указатели берутся из `ListColumnMap()` (порядок согласован).
- Для каждой пары из `PKColumnMap()` добавляется `Where(k+" = ?", v)`.
- Если реализован `WithGetInterceptorI` — вызывается `GetInterceptor(qb)`.
- `pgx.ErrNoRows` → `(false, nil)`. Иначе `(true, nil)` и поля модели заполнены через Scan.
- Пустой `ListColumnMap` → ошибка `no columns`.

## List

```go
func (s *ModelStore) List(ctx, params ListParams, itemConstructor func(add bool) ListModelI) (int64, error)
```

Порядок построения:
1. `SELECT FROM <table>`.
2. `Conditions` → `Where(map)`; каждый `ConditionExpressions[expr] = args` → `Where(expr, args...)`.
3. `itemConstructor(false)` → `ListColumnMap()` (вайтлист колонок).
4. Колонки: пересечение `params.Columns` с вайтлистом; если `Columns` пуст — все ключи карты.
   Нет валидных колонок → ошибка `no columns`.
5. `WithListInterceptorI.ListInterceptor(qb, params)` если реализован.
6. Если `WithTotalCount || OnlyCount`: добавляется `count(*)` (или `count(distinct (cols))` при
   `Distinct`), выполняется запрос count. При `OnlyCount` — возврат сразу. Иначе `RemoveColumns()`.
7. `Distinct()` при `params.Distinct`; затем `Columns(colNames...)`.
8. Пагинация при `PageSize > 0`: `Offset(Page*PageSize).Limit(PageSize)`.
9. Сортировка: `Sort == nil` → `DefaultSortColumns()`; `len(Sort) > 0` → `OrderBy(Sort...)`;
   `Sort == []` (не nil, пусто) → без ORDER BY.
10. Перебор строк: на каждую — `itemConstructor(true)` и `Scan` в указатели из `ListColumnMap`.

Возвращает `totalCount` (0, если count не запрашивали).

> Сортировка различает `nil` и пустой непустой срез: `nil` означает «возьми дефолт», явный пустой
> срез `[]string{}` — «без ORDER BY».

---

## Create / CreateMany

```go
func (s *ModelStore) Create(ctx, m CreateModelI) error
func (s *ModelStore) CreateMany(ctx, models []CreateModelI) error
```

SQL: `INSERT INTO <table> SET <CreateColumnMap>`.
- Если `ReturningColumnMap()` непуст → добавляется `RETURNING <cols>`, выполняется `QueryRow.Scan`
  в указатели (типично — получить сгенерированный `id`). Иначе `Exec`.
- `CreateMany` — через `pgx.Batch`; для каждой модели свой RETURNING-scan. Пустой срез → `nil`.
- PK **не добавляется автоматически** — то, что в `CreateColumnMap`, то и вставится.

## Update / UpdateMany

```go
func (s *ModelStore) Update(ctx, m UpdateModelI) error
func (s *ModelStore) UpdateMany(ctx, models []UpdateModelI) error
```

SQL: `UPDATE <table> SET <UpdateColumnMap> WHERE <pk> = $n ...`.
- `UpdateMany` — батч; пустой срез → `nil`.
- Частичный апдейт достигается тем, что `UpdateColumnMap` кладёт только не-nil поля.

## Delete / DeleteMany

```go
func (s *ModelStore) Delete(ctx, m DeleteModelI) error
func (s *ModelStore) DeleteMany(ctx, models []DeleteModelI) error
```

SQL: `DELETE FROM <table> WHERE <pk> = $n ...`. Батч-вариант аналогичен.

## UpdateOrCreate / UpdateOrCreateMany (UPSERT)

```go
func (s *ModelStore) UpdateOrCreate(ctx, m UpdateCreateModelI) error
func (s *ModelStore) UpdateOrCreateMany(ctx, models []UpdateCreateModelI) error
```

SQL: `INSERT INTO <table> as t SET <CreateColumnMap> ON CONFLICT (<pk-cols>) DO UPDATE SET <UpdateColumnMap>`.
- INSERT-часть = `CreateColumnMap()` **без автоматического добавления PK** → **модель обязана
  включать PK-колонки в `CreateColumnMap`**, иначе конфликт по PK не сработает (см. паттерн
  `UpsertWithPK` в `patterns.md`).
- UPDATE-часть = `UpdateColumnMap()` (обычно из неё PK исключают через `delete`).
- Таблица алиасится как `t` — на неё можно ссылаться в выражениях `UpdateColumnMap`.

## CreateIfNotExist / CreateIfNotExistMany

```go
func (s *ModelStore) CreateIfNotExist(ctx, m UpdateCreateModelI) error
func (s *ModelStore) CreateIfNotExistMany(ctx, models []UpdateCreateModelI) error
```

SQL: `INSERT INTO <table> SET <CreateColumnMap + PKColumnMap> ON CONFLICT (<pk-cols>) DO NOTHING`.
- В отличие от `UpdateOrCreate`, PK **добавляется автоматически** (мерж `PKColumnMap` в insert-карту).

---

## Интерсепторы

```go
type WithListInterceptorI interface {
    ListInterceptor(qb squirrel.SelectBuilder, params ListParams) squirrel.SelectBuilder
}
type WithGetInterceptorI interface {
    GetInterceptor(qb squirrel.SelectBuilder) squirrel.SelectBuilder
}
```

Реализуй на модели, чтобы донастроить запрос: JOIN, доп. WHERE, чтение `params.CustomConditions`.
Интерсептор должен **вернуть** изменённый builder.

```go
func (m *Select) ListInterceptor(qb squirrel.SelectBuilder, params mobone.ListParams) squirrel.SelectBuilder {
    if params.CustomConditions["with_active_orders"] != "" {
        qb = qb.Join("order o on o.item_id = t.id and o.status = 'active'")
    }
    return qb
}
```

---

## tools.ConstructSortColumns

```go
func ConstructSortColumns(allowedFields map[string]string, inputSort []string) []string
```

- `allowedFields`: внешнее_имя → SQL-выражение колонки. Пустое выражение → поле пропускается.
- `inputSort`: список полей; префикс `-` → `desc`. Неизвестные поля игнорируются.
- Пустой результат (или пустой вход) → `nil` (удобно для `if sort != nil`).

```go
allowed := map[string]string{"name": "user_name", "age": "user_age"}
tools.ConstructSortColumns(allowed, []string{"-name", "age", "unknown"})
// -> []string{"user_name desc", "user_age"}
```

---

## Строки-обёртки ошибок

`fail to build query`, `fail to query`, `fail to exec`, `fail to scan`, `rows.Err`, `no columns`,
`fail to close batch results`, `transaction function`, `transaction commit`,
`unable to begin transaction`. Батч-методы добавляют `(item N)`.
