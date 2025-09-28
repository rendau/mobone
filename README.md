# Mobone
### Пакет для работы с базой PostgreSQL

Пакет получает структуру через которую можно создавать, обновлять или удалять записи в базе
Для структур предусмотрены следующие интерфейсы:
- ListModelI для поиска товаров (ListColumnMap какие поля вернуть, DefaultSortColumns сортировка)
- GetModelI для получения одного товара (ListColumnMap какие поля вернуть, PKColumnMap primary поля, по ним идет поиск)
- CreateModelI для создания записи (CreateColumnMap какие поля записать в базу, ReturningColumnMap какие поля должны вернуться)
- UpdateModelI для обновления записи (UpdateColumnMap, какие поля обновить, PKColumnMap primary поля по которым будет поиск)

Для работы необходимо создать нужные структуры по этим интерфейсам

## Примеры структур
### для ListModelI и GetModelI
```go
package model

import "time"

type Select struct {
	Id        int
	Name      string
	Test      bool
	Json      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (m *Select) ListColumnMap() map[string]any {
	return map[string]any{
		"id":         &m.Id,
		"name":       &m.Name,
		"test":       &m.Test,
		"json":       &m.Json,
		"created_at": &m.CreatedAt,
		"updated_at": &m.UpdatedAt,
	}
}

func (m *Select) PKColumnMap() map[string]any {
	return map[string]any{
		"id": m.Id,
	}
}

func (m *Select) DefaultSortColumns() []string {
	return []string{"id"}
}
```

### для CreateModelI и UpdateModelI
```go
package model

import "time"

type Upsert struct {
	Id        int
	Name      string
	Test      bool
	Json      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (m *Upsert) CreateColumnMap() map[string]any {
	result := make(map[string]any, 5)

	result["name"] = m.Name
	result["test"] = m.Test
	result["json"] = m.Json
	result["created_at"] = m.CreatedAt
	result["updated_at"] = m.UpdatedAt

	return result
}

func (m *Upsert) UpdateColumnMap() map[string]any {
	return m.CreateColumnMap()
}

func (m *Upsert) ReturningColumnMap() map[string]any {
	return nil
}

func (m *Upsert) PKColumnMap() map[string]any {
	return map[string]any{
		"id": m.Id,
	}
}
```

## Примеры по использованию методов Mobone
### Получение одного элемента
```go
m := &model.Select{
    Id: 1,
}

modelStore := mobone.ModelStore{pgxPool, queryBuilder, "tableName"}

found, err := modelStore.Get(context.Background(), m)
```
после запроса в поля Select будут внесены записи в соответствии с ListColumnMap()

### Получение списка
```go
conditions := map[string]any{
    "Name": "Test Model",
}
conditionExps := map[string][]any{}

items := make([]*model.Select, 0)

modelStore := mobone.ModelStore{pgxPool, queryBuilder, "tableName"}

totalCount, err := modelStore.List(context.Background(), mobone.ListParams{
    Conditions:           conditions,
    ConditionExpressions: conditionExps,
    Page:                 0,
    PageSize:             5,
    WithTotalCount:       false,
    OnlyCount:            false,
    Sort:                 []string{"id"},
}, func(add bool) mobone.ListModelI {
    item := &model.Select{}
    if add {
        items = append(items, item)
    }
    return item
})
```
после запроса в поля Select будут внесены записи в соответствии с ListColumnMap()
* Page начинается с 0

### Создание записи
```go
upsertModel := &model.Upsert{
    Name:      "Test Model",
    Test:      true,
    Json:      `{"test": true}`,
    CreatedAt: time.Now(),
    UpdatedAt: time.Now(),
}

modelStore := mobone.ModelStore{pgxPool, queryBuilder, "tableName"}

err := modelStore.Create(context.Background(), upsertModel)
```
после записи в базу в Upsert вернутся поля указанные в его ReturningColumnMap()

### Обновление записи
```go
upsertModel := &model.Upsert{
    Id:   1,
    Name: "Test Model",
    Test: false,
    Json: `{"test": false}`,
}

modelStore := mobone.ModelStore{pgxPool, queryBuilder, "tableName"}

err := modelStore.Update(context.Background(), upsertModel)
```
Для обновления, поиск записи производится по полям указанным в PKColumnMap()

### Удаление записи
```go
modelStore := mobone.ModelStore{pgxPool, queryBuilder, "tableName"}

deleteModel := &model.Upsert{
    Id: 1,
}

err := modelStore.Delete(context.Background(), deleteModel)
```
Для удаления, поиск записи производится по полям указанным в PKColumnMap()
