# mobone — паттерны моделей, транзакции, подводные камни

## Две структуры на сущность: Select + Upsert

Рекомендуемый паттерн — отдельная структура для чтения и отдельная для записи.

### Select (чтение: List/Get)

Поля — обычные значения, в `ListColumnMap` отдаются **указатели** для Scan.

```go
type Select struct {
    Id        int
    CreatedAt time.Time
    UpdatedAt time.Time
    Name      string
    Flag      bool
    Contact   Contact // jsonb -> своя структура
}

func (m *Select) ListColumnMap() map[string]any {
    return map[string]any{
        "id":         &m.Id,
        "created_at": &m.CreatedAt,
        "updated_at": &m.UpdatedAt,
        "name":       &m.Name,
        "flag":       &m.Flag,
        "contact":    &m.Contact,
    }
}

func (m *Select) PKColumnMap() map[string]any   { return map[string]any{"id": m.Id} }
func (m *Select) DefaultSortColumns() []string  { return []string{"id"} }
```

### Upsert (запись: Create/Update) — частичные апдейты через указатели

Поля-указатели: `nil` → колонка не попадает в запрос. Так одна структура покрывает и полный, и
частичный апдейт.

```go
type Upsert struct {
    PKId int

    UpdatedAt *time.Time
    Name      *string
    Flag      *bool
    Contact   *ContactEdit
}

// INSERT-значения: только не-nil поля. PK сюда НЕ кладём (id генерит БД).
func (m *Upsert) CreateColumnMap() map[string]any {
    result := make(map[string]any, 5)
    if m.UpdatedAt != nil { result["updated_at"] = *m.UpdatedAt }
    if m.Name != nil      { result["name"] = *m.Name }
    if m.Flag != nil      { result["flag"] = *m.Flag }
    if m.Contact != nil   { result["contact"] = m.Contact }
    return result
}

// UPDATE-значения: те же, но без PK + частичный merge jsonb.
func (m *Upsert) UpdateColumnMap() map[string]any {
    result := m.CreateColumnMap()
    for k := range m.PKColumnMap() {
        delete(result, k)
    }
    if v, ok := result["contact"]; ok {
        result["contact"] = squirrel.Expr("contact || ?", v) // jsonb merge
    }
    return result
}

func (m *Upsert) ReturningColumnMap() map[string]any { return map[string]any{"id": &m.PKId} }
func (m *Upsert) PKColumnMap() map[string]any        { return map[string]any{"id": m.PKId} }
```

Использование:

```go
// Create — id вернётся в m.PKId через RETURNING
m := &Upsert{Name: new("Test"), Flag: new(true)}
err := store.Create(ctx, m)
newID := m.PKId

// Partial Update — обновится только name
err = store.Update(ctx, &Upsert{PKId: id, Name: new("Updated")})

// Batch partial update — у каждой модели свой набор полей
err = store.UpdateMany(ctx, []mobone.UpdateModelI{
    &Upsert{PKId: id1, Name: new("A")},
    &Upsert{PKId: id2, Flag: new(true)},
})

// Get
item := &Select{Id: id}
found, err := store.Get(ctx, item)
```

### UpsertWithPK — для UpdateOrCreate

⚠️ **`UpdateOrCreate` не добавляет PK в INSERT-часть автоматически.** Если использовать обычный
`Upsert` (где `CreateColumnMap` без `id`), `ON CONFLICT (id)` не сработает. Для upsert по известному
PK нужна структура, кладущая PK в `CreateColumnMap`:

```go
func (m *UpsertWithPK) CreateColumnMap() map[string]any {
    result := make(map[string]any, 6)
    result["id"] = m.PKId            // <-- PK включён в INSERT
    if m.Name != nil { result["name"] = *m.Name }
    // ... остальные поля
    return result
}
// UpdateColumnMap / ReturningColumnMap / PKColumnMap — как в Upsert (PK из update удаляется)
```

```go
// INSERT ... ON CONFLICT (id) DO UPDATE SET name = ...
err := store.UpdateOrCreate(ctx, &UpsertWithPK{PKId: id, Name: new("X")})
```

Для `CreateIfNotExist` PK добавляется библиотекой автоматически — туда подойдёт и обычный `Upsert`.

---

## jsonb-поля

Колонку `jsonb` маппят на Go-структуру напрямую (pgx сам (де)сериализует):

```go
type Contact struct {
    Phone string `json:"phone"`
    Email string `json:"email"`
}
type ContactEdit struct {
    Phone *string `json:"phone,omitempty"` // omitempty для частичного merge
    Email *string `json:"email,omitempty"`
}
```

- Чтение: `"contact": &m.Contact` в `ListColumnMap`.
- Полная перезапись: `result["contact"] = m.Contact`.
- Частичный merge: `result["contact"] = squirrel.Expr("contact || ?", m.Contact)` — Postgres
  смержит JSON, перезаписав только присутствующие ключи (поэтому `omitempty`).

---

## Транзакции

`TransactionManager` кладёт `pgx.Tx` в `context` — все вызовы store внутри `TxFn` идут на одной
транзакции. Вложенные `TxFn` переиспользуют уже открытую транзакцию из ctx (один commit снаружи).

```go
txM := mobone.NewTransactionManager(pool)

store := mobone.ModelStore{
    Con:                pool,
    TransactionManager: txM, // ОБЯЗАТЕЛЬНО прокинуть в store, иначе tx не подхватится
    QB:                 squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
    TableName:          "item",
}

err := txM.TxFn(ctx, func(ctx context.Context) error {
    if err := store.Update(ctx, m1); err != nil {
        return err // <-- любой возврат ошибки => Rollback
    }
    return store.Create(ctx, m2)
}) // успех => Commit
```

- `TxFn` сам делает `defer Rollback` и `Commit` в конце. Возврат ошибки из функции → откат.
- **Передавай `ctx` из колбэка** во все вызовы store — именно в нём лежит транзакция.
- Если несколько store-ов должны работать в одной транзакции — у всех должен быть один `txM`.

---

## Чеклист подводных камней

- **Указатель vs значение.** Scan-карты (`ListColumnMap`, `ReturningColumnMap`) — указатели
  (`&m.X`). Where/Set-карты (`PKColumnMap`, `CreateColumnMap`, `UpdateColumnMap`) — значения.
  Перепутал — Scan упадёт или запишется мусор.
- **`add == true` в List.** Добавляй элемент в срез только при `add == true`, иначе попадёт лишний
  пустой элемент от метаданного вызова `itemConstructor(false)`.
- **PlaceholderFormat(Dollar).** Без него squirrel сгенерит `?`, и pgx не выполнит запрос.
- **`UpdateOrCreate` требует PK в `CreateColumnMap`** (см. UpsertWithPK). `CreateIfNotExist` — нет.
- **`Sort`: nil ≠ `[]string{}`.** `nil` → `DefaultSortColumns()`; пустой непустой срез → без ORDER BY.
- **Несуществующие `Columns` молча отбрасываются**; пустой результат → ошибка `no columns`.
- **`Get` не возвращает ошибку при отсутствии строки** — проверяй `found bool`, а не `err`.
- **Имена колонок не экранируются.** Для пользовательской сортировки — `tools.ConstructSortColumns`
  с вайтлистом; не подставляй сырой ввод в `Sort`/`TableName`/ключи карт.
- **Транзакция не подхватится**, если `TransactionManager` не задан в `ModelStore` или в store
  передан не тот `ctx`.
