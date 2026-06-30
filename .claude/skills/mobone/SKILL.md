---
name: mobone
description: >
  Минималистичный слой доступа к PostgreSQL на Go поверх pgx/pgxpool + Masterminds/squirrel
  (github.com/mechta-market/mobone/v2). Даёт generic CRUD через ModelStore и интерфейсы моделей
  (ListModelI, GetModelI, CreateModelI, UpdateModelI, DeleteModelI, UpdateCreateModelI):
  Get/List с пагинацией, сортировкой и условиями; Create/Update/Delete и их батч-варианты
  (CreateMany/UpdateMany/DeleteMany через pgx.Batch); UpdateOrCreate (ON CONFLICT DO UPDATE),
  CreateIfNotExist (ON CONFLICT DO NOTHING); транзакции через TransactionManager (tx в context);
  безопасная сборка ORDER BY через tools.ConstructSortColumns.
  Применяй этот скилл ВСЕГДА когда задача касается: mobone, ModelStore, репозитория поверх pgx,
  написания select.go/upsert.go, *ColumnMap, ListParams, getConditions, ListInterceptor,
  upsert/batch операций, squirrel-билдера запросов, транзакций pgx, маппинга колонок модели.
  Триггеры: mobone, ModelStore, ListModelI, GetModelI, CreateModelI, UpdateModelI, ListParams,
  ListColumnMap, PKColumnMap, CreateColumnMap, UpdateColumnMap, ReturningColumnMap, UpdateOrCreate,
  CreateIfNotExist, ListInterceptor, ConstructSortColumns, TransactionManager, TxFn.
user-invocable: true
license: MIT
metadata:
  author: mechta-market
  version: "1.0.0"
---

# mobone — слой доступа к PostgreSQL (pgx + squirrel)

`github.com/mechta-market/mobone/v2` — тонкий generic CRUD-слой. Работает **не со структурами
напрямую, а с интерфейсами моделей**, которые отдают карты `имя_колонки → значение | указатель`.
Это убирает рефлексию и шаблонный SQL, оставляя полный контроль над запросами через `squirrel`.

- **Driver:** `pgx/v5` + `pgxpool` (`Con *pgxpool.Pool`).
- **Builder:** `Masterminds/squirrel`. **Обязательно** `PlaceholderFormat(squirrel.Dollar)` для Postgres.
- **Go 1.24+.**

Полный справочник по API и операциям — `references/api-reference.md`.
Паттерны моделей, транзакции и подводные камни — `references/patterns.md`.

---

## Инициализация ModelStore

`ModelStore` — одна структура на одну таблицу. Создавай по одному store на сущность.

```go
import (
    "github.com/Masterminds/squirrel"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/mechta-market/mobone/v2"
)

qb := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar) // ВСЕГДА Dollar для PG

store := mobone.ModelStore{
    Con:                pool,  // *pgxpool.Pool
    TransactionManager: txM,   // опционально, для tx через context (см. ниже)
    QB:                 qb,
    TableName:          "item", // имя таблицы в единственном числе
}
```

`GetConnection(ctx)`: если задан `TransactionManager` и в `ctx` лежит активная транзакция —
вернётся `tx`, иначе пул. Так одни и те же вызовы store автоматически работают внутри транзакции.

---

## Интерфейсы моделей (что и для какой операции реализовать)

Ключи карт — это **имена колонок / SQL-выражений**. Значения:
- **указатель** на поле (`&m.Id`) — для Scan (чтение: `ListColumnMap`, `ReturningColumnMap`);
- **значение** (`m.Id`) — для Where/SetMap (`PKColumnMap`, `CreateColumnMap`, `UpdateColumnMap`).

| Метод | Назначение | Значения |
|---|---|---|
| `ListColumnMap() map[string]any` | колонки для Select/Scan | указатели |
| `DefaultSortColumns() []string` | ORDER BY по умолчанию | — |
| `PKColumnMap() map[string]any` | условие по PK (Where) | значения |
| `CreateColumnMap() map[string]any` | значения для INSERT (SetMap) | значения |
| `UpdateColumnMap() map[string]any` | значения для UPDATE (SetMap) | значения |
| `ReturningColumnMap() map[string]any` | поля для RETURNING | указатели |

Композиция интерфейсов:
- `ListModelI` = `ListColumnMap` + `DefaultSortColumns`
- `GetModelI` = `ListColumnMap` + `PKColumnMap`
- `CreateModelI` = `CreateColumnMap` + `ReturningColumnMap`
- `UpdateModelI` = `UpdateColumnMap` + `PKColumnMap`
- `DeleteModelI` = `PKColumnMap`
- `UpdateCreateModelI` = `UpdateModelI` + `CreateModelI` (для upsert / insert-if-not-exist)
- опционально `WithListInterceptorI` (`ListInterceptor`), `WithGetInterceptorI` (`GetInterceptor`)

На практике достаточно **двух структур на сущность**: `Select` (чтение) и `Upsert` (запись) —
см. `references/patterns.md`.

---

## Операции ModelStore (сигнатуры)

```go
Get(ctx, m GetModelI) (found bool, err error)
List(ctx, params ListParams, itemConstructor func(add bool) ListModelI) (totalCount int64, err error)

Create(ctx, m CreateModelI) error
Update(ctx, m UpdateModelI) error
Delete(ctx, m DeleteModelI) error

UpdateOrCreate(ctx, m UpdateCreateModelI) error    // ON CONFLICT (pk) DO UPDATE SET ...
CreateIfNotExist(ctx, m UpdateCreateModelI) error  // ON CONFLICT (pk) DO NOTHING

// батч-варианты через pgx.Batch (пустой срез — no-op, nil):
CreateMany(ctx, []CreateModelI) error
UpdateMany(ctx, []UpdateModelI) error
DeleteMany(ctx, []DeleteModelI) error
UpdateOrCreateMany(ctx, []UpdateCreateModelI) error
CreateIfNotExistMany(ctx, []UpdateCreateModelI) error
```

### List: itemConstructor

`List` вызывает `itemConstructor(false)` один раз (метаданные колонок) и затем
`itemConstructor(true)` **на каждую прочитанную строку**. Добавляй элемент в коллекцию только
когда `add == true`:

```go
var items []*model.Select
total, err := store.List(ctx, mobone.ListParams{
    Page: 0, PageSize: 20,
    Sort:           []string{"id desc"},
    WithTotalCount: true,
}, func(add bool) mobone.ListModelI {
    it := &model.Select{}
    if add {
        items = append(items, it)
    }
    return it
})
```

---

## ListParams

```go
type ListParams struct {
    Conditions           map[string]any      // Where(map): col = value (AND)
    ConditionExpressions map[string][]any    // Where("col ilike ?", args...)
    Distinct             bool
    Columns              []string            // подмножество ListColumnMap; пусто = все колонки
    Page                 int64               // нумерация с 0; Offset = Page*PageSize
    PageSize             int64               // 0 = без LIMIT/OFFSET
    WithTotalCount       bool                // вернуть count вместе с данными
    OnlyCount            bool                // вернуть только count, без выборки строк
    Sort                 []string            // ORDER BY; nil = DefaultSortColumns(); [] = без сортировки
    CustomConditions     map[string]string   // ваши флаги, читаются в ListInterceptor
}
```

Условия (можно комбинировать):

```go
params := mobone.ListParams{
    Conditions:           map[string]any{"flag": true, "status": "active"},
    ConditionExpressions: map[string][]any{"name ilike ?": {"%test%"}},
}
```

`Columns`, которых нет в `ListColumnMap()`, молча отбрасываются. Если в итоге колонок 0 — ошибка
`no columns`.

---

## tools.ConstructSortColumns — безопасный ORDER BY

Собирает `Sort` из вайтлиста, защищая от SQL-инъекций в имена колонок. Префикс `-` = DESC,
неразрешённые поля игнорируются, пустой результат → `nil`.

```go
import "github.com/mechta-market/mobone/v2/tools"

allowed := map[string]string{"name": "user_name", "age": "user_age"}
params.Sort = tools.ConstructSortColumns(allowed, []string{"-name", "age"})
// -> []string{"user_name desc", "user_age"}
```

---

## Обработка ошибок

Все методы оборачивают ошибки (`fmt.Errorf("...: %w")`) строками: `fail to build query`,
`fail to query`, `fail to exec`, `fail to scan`. Используй `errors.Is`/`errors.As` для разворота.
`Get` сам ловит `pgx.ErrNoRows` и возвращает `found == false` (а не ошибку) — проверяй булев флаг.

---

## Ключевые правила

- **Всегда** `PlaceholderFormat(squirrel.Dollar)` для Postgres.
- В `ListColumnMap`/`ReturningColumnMap` — **указатели** на поля; в `PKColumnMap`/`*ColumnMap`
  для записи — **значения**.
- В `List` добавляй элемент в срез только при `add == true`.
- Имена таблиц и колонок не экранируются автоматически — для пользовательской сортировки
  используй `tools.ConstructSortColumns`.
- ⚠️ `UpdateOrCreate` берёт INSERT-часть из `CreateColumnMap()` **как есть** (PK не добавляется
  автоматически) — для upsert модель должна включать PK в `CreateColumnMap`. А `CreateIfNotExist`
  PK добавляет сам. Подробнее — `references/patterns.md`.
