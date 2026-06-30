# mobone — минималистичный слой для работы с PostgreSQL через pgx и Squirrel

mobone упрощает CRUD-операции и списочные выборки для моделей на Go, используя:
- pgx/pgxpool как драйвер PostgreSQL
- Masterminds/squirrel как билдер SQL с плейсхолдерами
- Простые интерфейсы моделей для маппинга колонок и значений

Подходит для приложений, где нужно быстро собрать надежный слой доступа к данным с минимальным шаблонным кодом.

- Требования: Go 1.24, pgx v5, squirrel.
- Рекомендуется использовать формат плейсхолдеров Dollar для PostgreSQL: StatementBuilder.PlaceholderFormat(squirrel.Dollar)

## Установка

```shell script
go get github.com/mechta-market/mobone/v2
```


## Быстрый старт

### Инициализация хранилища

```textmate
// Go
import (
  "context"

  "github.com/Masterminds/squirrel"
  "github.com/jackc/pgx/v5/pgxpool"
  "github.com/mechta-market/mobone/v2"
)

func NewStore(pool *pgxpool.Pool) mobone.ModelStore {
  qb := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
  return mobone.ModelStore{
    Con:       pool,
    QB:        qb,
    TableName: "your_table",
  }
}
```


### Интерфейсы моделей

mobone работает не с структурами напрямую, а с интерфейсами, которые возвращают:
- карту колонок => адрес полей для Scan (ListModelI, GetModelI)
- карту значений для INSERT/UPDATE (CreateModelI, UpdateModelI)
- карту PK-условий (GetModelI, UpdateModelI, DeleteModelI)
- опционально возвращаемые поля (CreateModelI.ReturningColumnMap)
- опциональные перехватчики запросов (WithListInterceptorI, WithGetInterceptorI)

Ключи карт — имена колонок/выражений в SQL, значения — либо конкретные значения (для SetMap/Where), либо указатели на поля (для Scan).

## Интерфейсы

- TransactionManagerI
    - GetConnection(ctx) ConnectionI
    - TxFn(ctx, func(ctx) error) error — оборачивает функцию в транзакцию

- ConnectionI
    - Exec(ctx, sql, args...) (pgconn.CommandTag, error)
    - Query(ctx, sql, args...) (pgx.Rows, error)
    - QueryRow(ctx, sql, args...) pgx.Row
    - SendBatch(ctx, b *pgx.Batch) pgx.BatchResults

- ListModelI
    - ListColumnMap() map[string]any — колонки для Select/Scan
    - DefaultSortColumns() []string — сортировка по умолчанию

- GetModelI
    - ListColumnMap()
    - PKColumnMap() map[string]any — условия по PK

- CreateModelI
    - CreateColumnMap() map[string]any — значения для INSERT
    - ReturningColumnMap() map[string]any — поля для RETURNING

- UpdateModelI
    - UpdateColumnMap() map[string]any — значения для UPDATE
    - PKColumnMap()

- UpdateCreateModelI
    - объединяет UpdateModelI + CreateModelI (для upsert/insert-if-not-exists)

- DeleteModelI
    - PKColumnMap()

- WithListInterceptorI
    - ListInterceptor(qb, params) — тонкая настройка SELECT

- WithGetInterceptorI
    - GetInterceptor(qb)

## ModelStore: операции

- Create(ctx, m CreateModelI) error
- CreateMany(ctx, models []CreateModelI) error — батч INSERT через pgx.Batch
- Update(ctx, m UpdateModelI) error
- UpdateMany(ctx, models []UpdateModelI) error — батч UPDATE через pgx.Batch
- UpdateOrCreate(ctx, m UpdateCreateModelI) error — ON CONFLICT DO UPDATE
- UpdateOrCreateMany(ctx, models []UpdateCreateModelI) error — батч upsert через pgx.Batch
- CreateIfNotExist(ctx, m UpdateCreateModelI) error — ON CONFLICT DO NOTHING
- CreateIfNotExistMany(ctx, models []UpdateCreateModelI) error — батч insert-if-not-exist через pgx.Batch
- Delete(ctx, m DeleteModelI) error
- DeleteMany(ctx, models []DeleteModelI) error — батч DELETE через pgx.Batch
- Get(ctx, m GetModelI) (found bool, err error)
- List(ctx, params ListParams, itemConstructor func(add bool) ListModelI) (totalCount int64, err error)

ListParams:
- Conditions map[string]any — простые условия Where(map)
- ConditionExpressions map[string][]any — выражения Where("a = ? and b > ?", args...)
- Distinct bool
- Columns []string — какие колонки вернуть (по умолчанию — все из ListColumnMap)
- Page, PageSize int64 — пагинация (Offset = Page*PageSize)
- WithTotalCount bool — вместе с данными вернуть count
- OnlyCount bool — вернуть только count (без данных)
- Sort []string — список ORDER BY (если пусто — берется DefaultSortColumns)
- CustomConditions map[string]string — для ваших кастомизаций (используйте в перехватчиках)

## Транзакции

TransactionManager прокидывает pgx.Tx через context, чтобы ModelStore автоматически использовал один и тот же ConnectionI (tx вместо пула) внутри TxFn.

```textmate
// Go
txM := mobone.NewTransactionManager(pool)

store := mobone.ModelStore{
  Con:                pool,
  TransactionManager: txM,
  QB:                 squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
  TableName:          "your_table",
}

err := txM.TxFn(context.Background(), func(ctx context.Context) error {
  // все вызовы store внутри будут на одной транзакции
  // например: return store.Update(ctx, updateModel)
  return nil
})
```


## Пример модели

```textmate
// Go
type Contact struct {
  Phone string
  Email string
}

type ContactEdit struct {
  Phone *string
  Email *string
}

type Item struct {
  Id        int
  Name      string
  Flag      bool
  Contact   Contact
  CreatedAt time.Time
  UpdatedAt time.Time
}

// Для списков/получения
func (m *Item) ListColumnMap() map[string]any {
  return map[string]any{
    "id":         &m.Id,
    "name":       &m.Name,
    "flag":       &m.Flag,
    "contact":    &m.Contact,
    "created_at": &m.CreatedAt,
    "updated_at": &m.UpdatedAt,
  }
}

func (m *Item) PKColumnMap() map[string]any {
  return map[string]any{"id": m.Id}
}

func (m *Item) DefaultSortColumns() []string { return []string{"id"} }

// Для создания/обновления
type ItemUpsert struct {
  PKId      int
  Name      *string
  Flag      *bool
  Contact   *ContactEdit
  UpdatedAt *time.Time
}

func (u *ItemUpsert) CreateColumnMap() map[string]any {
  res := map[string]any{}
  if u.Name != nil { res["name"] = *u.Name }
  if u.Flag != nil { res["flag"] = *u.Flag }
  if u.Contact != nil { res["contact"] = u.Contact }
  if u.UpdatedAt != nil { res["updated_at"] = *u.UpdatedAt }
  return res
}

func (u *ItemUpsert) UpdateColumnMap() map[string]any {
  res := u.CreateColumnMap()
  // не обновляем PK-колонки
  delete(res, "id")
  // пример частичного merge jsonb (через squirrel.Expr)
  if v, ok := res["contact"]; ok {
    res["contact"] = squirrel.Expr("contact || ?", v)
  }
  return res
}

func (u *ItemUpsert) ReturningColumnMap() map[string]any {
  return map[string]any{"id": &u.PKId}
}

func (u *ItemUpsert) PKColumnMap() map[string]any {
  return map[string]any{"id": u.PKId}
}
```


## Примеры операций

### Create

```textmate
// Go
store := mobone.ModelStore{Con: pool, QB: qb, TableName: "items"}

name := "Test"
flag := true
create := &ItemUpsert{
  Name: &name,
  Flag: &flag,
}
err := store.Create(ctx, create)
if err != nil { /* handle */ }
id := create.PKId
```


### Get

```textmate
// Go
item := &Item{Id: id}
found, err := store.Get(ctx, item)
if err != nil { /* handle */ }
if !found { /* not found */ }
// item заполнен из БД
```


### Update

```textmate
// Go
newName := "Updated"
now := time.Now()
upd := &ItemUpsert{
  PKId:      id,
  Name:      &newName,
  UpdatedAt: &now,
}
err := store.Update(ctx, upd)
```

### UpdateMany

```textmate
// Go
name := "Updated 1"
flag := true
err := store.UpdateMany(ctx, []mobone.UpdateModelI{
  &ItemUpsert{PKId: id1, Name: &name}, // partial: обновится только name
  &ItemUpsert{PKId: id2, Flag: &flag}, // partial: обновится только flag
})
```


### Delete

```textmate
// Go
err := store.Delete(ctx, &ItemUpsert{PKId: id})
```


### List с пагинацией и сортировкой

```textmate
// Go
var items []*Item
total, err := store.List(ctx, mobone.ListParams{
  Page:       0,
  PageSize:   20,
  Sort:       []string{"id desc"},
  WithTotalCount: true,
}, func(add bool) mobone.ListModelI {
  it := &Item{}
  if add { items = append(items, it) }
  return it
})
// items заполнен, total содержит количество (если WithTotalCount)
```


### List с ограниченным набором колонок

```textmate
// Go
var items []*Item
_, err := store.List(ctx, mobone.ListParams{
  Columns: []string{"id", "name"}, // берутся только разрешенные в ListColumnMap()
  PageSize: 50,
}, func(add bool) mobone.ListModelI {
  it := &Item{}
  if add { items = append(items, it) }
  return it
})
```


### Только подсчет (без данных)

```textmate
// Go
count, err := store.List(ctx, mobone.ListParams{
  OnlyCount: true,
}, func(add bool) mobone.ListModelI {
  return &Item{}
})
```


### Условия

```textmate
// Go
// Простой Where через карту (эквивалент field = value)
params := mobone.ListParams{
  Conditions: map[string]any{
    "flag": true,
  },
}

// Произвольные выражения с плейсхолдерами
params.ConditionExpressions = map[string][]any{
  "name ilike ?": {"%test%"},
}
```


### Interceptor для списков

```textmate
// Go
type ItemList struct{ Item }

func (m *ItemList) ListInterceptor(qb squirrel.SelectBuilder, params mobone.ListParams) squirrel.SelectBuilder {
  // например, форсируем join при определенных колонках
  if len(params.Columns) == 1 && params.Columns[0] == "id" {
    qb = qb.CrossJoin("(select 1) as s")
  }
  return qb
}
```


## Upsert и Insert-if-not-exists

```textmate
// Go
// ON CONFLICT (id) DO UPDATE SET ...
err := store.UpdateOrCreate(ctx, &ItemUpsert{
  PKId: id,
  Name: &newName,
})

// ON CONFLICT (id) DO NOTHING
err := store.CreateIfNotExist(ctx, &ItemUpsert{
  PKId: id,
  Name: &newName,
})
```


## Утилиты сортировки

Пакет tools содержит функцию для безопасной сборки ORDER BY из whitelisted полей.

```textmate
// Go
import "github.com/mechta-market/mobone/v2/tools"

allowed := map[string]string{
  "name": "user_name",
  "age":  "user_age",
}

order := tools.ConstructSortColumns(allowed, []string{"-name", "age"})
// -> []string{"user_name desc", "user_age"}

// Используйте в ListParams.Sort:
params.Sort = order
```


Особенности:
- Игнорирует неразрешенные поля
- Поддерживает префикс "-" для DESC
- Возвращает nil, если итог пуст (удобно для проверки)

## Рекомендации

- Всегда используйте PlaceholderFormat(squirrel.Dollar) с PostgreSQL.
- В ReturningColumnMap возвращайте указатели на поля вашей структуры.
- В List используйте itemConstructor(add bool): добавляйте элемент в коллекцию только когда add == true (когда фактически прочитана строка).
- Для JSONB-merge применяйте squirrel.Expr в UpdateColumnMap.

## Обработка ошибок

mobone возвращает ошибки с оберткой контекста:
- "fail to build query"
- "fail to query"
- "fail to exec"
- "transaction function"
- "transaction commit"

Используйте errors.Is для проверки pgx.ErrNoRows в Get.
