package tests

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/require"

	"github.com/rendau/mobone/v2"
	"github.com/rendau/mobone/v2/tests/model"
)

const tableName = "tests"

var dbCon *Con
var queryBuilder = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

func TestMain(m *testing.M) {
	dbName := os.Getenv("TEST_DB_NAME")
	if dbName == "" {
		dbName = "mobone"
	}

	err := recreateDB(dbName)
	if err != nil {
		log.Printf("recreateDB: %v\n", err)
		os.Exit(1)
	}

	dbCon, err = NewCon(dbName)
	if err != nil {
		log.Printf("NewCon: %v\n", err)
		os.Exit(1)
	}

	err = initSchema(dbCon)
	if err != nil {
		log.Printf("initSchema: %v\n", err)
		os.Exit(1)
	}

	// RUN TESTS
	exitCode := m.Run()

	dbCon.Close()

	os.Exit(exitCode)
}

func recreateDB(dbName string) error {
	ctx := context.Background()

	// recreate database
	{
		con, err := NewCon("")
		if err != nil {
			return fmt.Errorf("NewCon: %w", err)
		}
		defer con.Close()

		_, err = con.pool.Exec(ctx, fmt.Sprintf("drop database if exists %s", dbName))
		if err != nil {
			return fmt.Errorf("unable to drop database: %w", err)
		}

		_, err = con.pool.Exec(ctx, fmt.Sprintf("create database %s", dbName))
		if err != nil {
			return fmt.Errorf("unable to create database: %w", err)
		}
	}

	return nil
}

func initSchema(con *Con) error {
	ctx := context.Background()

	_, err := con.pool.Exec(ctx, `
		CREATE TABLE `+tableName+` (
		    id SERIAL PRIMARY KEY,
		    created_at timestamptz not null default now(),
		    updated_at timestamptz not null default now(),
		    name text not null default '',
		    flag boolean not null default false,
		    contact jsonb not null default '{}'
		)
	`)
	if err != nil {
		return fmt.Errorf("unable to create table: %w", err)
	}

	return nil
}

func TestCreate(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	item := &model.Select{
		Name: "Test Model",
		Flag: true,
		Contact: model.Contact{
			Phone: "123456789",
			Email: "test@example.com",
		},
	}

	createModel := &model.Upsert{
		Name: &item.Name,
		Flag: &item.Flag,
		Contact: &model.ContactEdit{
			Phone: &item.Contact.Phone,
			Email: &item.Contact.Email,
		},
	}
	err = modelStore.Create(ctx, createModel)
	require.NoError(t, err)
	require.Greater(t, createModel.PKId, 0)
	item.Id = createModel.PKId

	dbItem := &model.Select{Id: item.Id}
	found, err := modelStore.Get(ctx, dbItem)
	require.NoError(t, err)
	require.True(t, found)
	require.WithinDuration(t, time.Now(), dbItem.CreatedAt, 30*time.Millisecond)
	require.WithinDuration(t, time.Now(), dbItem.UpdatedAt, 30*time.Millisecond)
	dbItem.CreatedAt = time.Time{}
	dbItem.UpdatedAt = time.Time{}
	require.Equal(t, item, dbItem)

	dbItems := make([]*model.Select, 0, 3)
	_, err = modelStore.List(ctx, mobone.ListParams{
		PageSize: 10,
	}, func(add bool) mobone.ListModelI {
		x := &model.Select{}
		if add {
			dbItems = append(dbItems, x)
		}
		return x
	})
	require.NoError(t, err)
	require.Len(t, dbItems, 1)
	dbItem = dbItems[0]
	dbItem.CreatedAt = time.Time{}
	dbItem.UpdatedAt = time.Time{}
	require.Equal(t, item, dbItem)
}

func TestCreateMany(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	name1 := "Test Model 1"
	flag1 := true
	createModel1 := &model.Upsert{
		Name: &name1,
		Flag: &flag1,
	}

	name2 := "Test Model 2"
	flag2 := false
	createModel2 := &model.Upsert{
		Name: &name2,
		Flag: &flag2,
	}

	err = modelStore.CreateMany(ctx, []mobone.CreateModelI{
		createModel1,
		createModel2,
	})
	require.NoError(t, err)
	require.Greater(t, createModel1.PKId, 0)
	require.Greater(t, createModel2.PKId, 0)
	require.NotEqual(t, createModel1.PKId, createModel2.PKId)

	dbItem1 := &model.Select{Id: createModel1.PKId}
	found, err := modelStore.Get(ctx, dbItem1)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, name1, dbItem1.Name)
	require.Equal(t, flag1, dbItem1.Flag)

	dbItem2 := &model.Select{Id: createModel2.PKId}
	found, err = modelStore.Get(ctx, dbItem2)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, name2, dbItem2.Name)
	require.Equal(t, flag2, dbItem2.Flag)
}

func TestCreateIfNotExistMany(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	existingName := "Existing"
	err = modelStore.CreateIfNotExist(ctx, &model.Upsert{
		PKId: 101,
		Name: &existingName,
	})
	require.NoError(t, err)

	existingNameIgnored := "Existing Changed"
	newName := "New"
	err = modelStore.CreateIfNotExistMany(ctx, []mobone.UpdateCreateModelI{
		&model.Upsert{
			PKId: 101,
			Name: &existingNameIgnored,
		},
		&model.Upsert{
			PKId: 102,
			Name: &newName,
		},
	})
	require.NoError(t, err)

	dbItem := &model.Select{Id: 101}
	found, err := modelStore.Get(ctx, dbItem)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, existingName, dbItem.Name)

	dbItem = &model.Select{Id: 102}
	found, err = modelStore.Get(ctx, dbItem)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, newName, dbItem.Name)
}

func TestUpdate(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	item := &model.Select{
		Name: "Test Model",
		Flag: true,
		Contact: model.Contact{
			Phone: "123456789",
			Email: "test@example.com",
		},
	}

	createModel := &model.Upsert{
		Name: &item.Name,
		Flag: &item.Flag,
		Contact: &model.ContactEdit{
			Phone: &item.Contact.Phone,
			Email: &item.Contact.Email,
		},
	}
	err = modelStore.Create(ctx, createModel)
	require.NoError(t, err)
	item.Id = createModel.PKId

	item.UpdatedAt = time.Now().Add(-time.Hour)
	item.Name = "Test Model changed"
	item.Flag = false
	item.Contact.Phone = "987654321"
	item.Contact.Email = "changed@example.com"

	updateModel := &model.Upsert{
		PKId:      item.Id,
		UpdatedAt: &item.UpdatedAt,
		Name:      &item.Name,
		Flag:      &item.Flag,
		Contact: &model.ContactEdit{
			Phone: &item.Contact.Phone,
			Email: &item.Contact.Email,
		},
	}
	err = modelStore.Update(ctx, updateModel)
	require.NoError(t, err)
	require.Greater(t, updateModel.PKId, 0)
	item.Id = updateModel.PKId

	dbItem := &model.Select{Id: item.Id}
	found, err := modelStore.Get(ctx, dbItem)
	require.NoError(t, err)
	require.True(t, found)
	require.WithinDuration(t, time.Now(), dbItem.CreatedAt, 30*time.Millisecond)
	require.WithinDuration(t, item.UpdatedAt, dbItem.UpdatedAt, 30*time.Millisecond)
	dbItem.CreatedAt = time.Time{}
	dbItem.UpdatedAt = item.UpdatedAt
	require.Equal(t, item, dbItem)
}

func TestUpdateOrCreateMany(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	initialName := "Initial"
	err = modelStore.Create(ctx, &model.UpsertWithPK{
		PKId: 201,
		Name: &initialName,
	})
	require.NoError(t, err)

	updatedName := "Updated"
	createdName := "Created"
	err = modelStore.UpdateOrCreateMany(ctx, []mobone.UpdateCreateModelI{
		&model.UpsertWithPK{
			PKId: 201,
			Name: &updatedName,
		},
		&model.UpsertWithPK{
			PKId: 202,
			Name: &createdName,
		},
	})
	require.NoError(t, err)

	dbItem := &model.Select{Id: 201}
	found, err := modelStore.Get(ctx, dbItem)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, updatedName, dbItem.Name)

	dbItem = &model.Select{Id: 202}
	found, err = modelStore.Get(ctx, dbItem)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, createdName, dbItem.Name)
}

func TestUpdateMany(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	name1 := "Item 1"
	flag1 := true
	createModel1 := &model.Upsert{
		Name: &name1,
		Flag: &flag1,
	}
	err = modelStore.Create(ctx, createModel1)
	require.NoError(t, err)

	name2 := "Item 2"
	flag2 := false
	createModel2 := &model.Upsert{
		Name: &name2,
		Flag: &flag2,
	}
	err = modelStore.Create(ctx, createModel2)
	require.NoError(t, err)

	updatedName1 := "Item 1 updated"
	updatedFlag2 := true

	err = modelStore.UpdateMany(ctx, []mobone.UpdateModelI{
		&model.Upsert{
			PKId: createModel1.PKId,
			Name: &updatedName1,
		},
		&model.Upsert{
			PKId: createModel2.PKId,
			Flag: &updatedFlag2,
		},
	})
	require.NoError(t, err)

	dbItem1 := &model.Select{Id: createModel1.PKId}
	found, err := modelStore.Get(ctx, dbItem1)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, updatedName1, dbItem1.Name)
	require.Equal(t, flag1, dbItem1.Flag)

	dbItem2 := &model.Select{Id: createModel2.PKId}
	found, err = modelStore.Get(ctx, dbItem2)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, name2, dbItem2.Name)
	require.Equal(t, updatedFlag2, dbItem2.Flag)
}

func TestTransactionUpdateMany(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	bgCtx := context.Background()

	txM := mobone.NewTransactionManager(dbCon.pool)

	modelStore := mobone.ModelStore{
		Con:                dbCon.pool,
		TransactionManager: txM,
		QB:                 queryBuilder,
		TableName:          tableName,
	}

	name1 := "Tx Item 1"
	flag1 := true
	createModel1 := &model.Upsert{
		Name: &name1,
		Flag: &flag1,
	}
	err = modelStore.Create(bgCtx, createModel1)
	require.NoError(t, err)

	name2 := "Tx Item 2"
	flag2 := false
	createModel2 := &model.Upsert{
		Name: &name2,
		Flag: &flag2,
	}
	err = modelStore.Create(bgCtx, createModel2)
	require.NoError(t, err)

	updatedName1 := "Tx Item 1 updated"
	updatedFlag2 := true
	txFnErr := txM.TxFn(bgCtx, func(ctx context.Context) error {
		return modelStore.UpdateMany(ctx, []mobone.UpdateModelI{
			&model.Upsert{
				PKId: createModel1.PKId,
				Name: &updatedName1,
			},
			&model.Upsert{
				PKId: createModel2.PKId,
				Flag: &updatedFlag2,
			},
		})
	})
	require.NoError(t, txFnErr)

	dbItem1 := &model.Select{Id: createModel1.PKId}
	found, err := modelStore.Get(bgCtx, dbItem1)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, updatedName1, dbItem1.Name)
	require.Equal(t, flag1, dbItem1.Flag)

	dbItem2 := &model.Select{Id: createModel2.PKId}
	found, err = modelStore.Get(bgCtx, dbItem2)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, name2, dbItem2.Name)
	require.Equal(t, updatedFlag2, dbItem2.Flag)

	rollbackName1 := "Tx Item 1 rolled back"
	rollbackFlag2 := false
	txFnErr = txM.TxFn(bgCtx, func(ctx context.Context) error {
		err = modelStore.UpdateMany(ctx, []mobone.UpdateModelI{
			&model.Upsert{
				PKId: createModel1.PKId,
				Name: &rollbackName1,
			},
			&model.Upsert{
				PKId: createModel2.PKId,
				Flag: &rollbackFlag2,
			},
		})
		if err != nil {
			return err
		}
		return fmt.Errorf("test error")
	})
	require.NotNil(t, txFnErr, "TxFn should return error")
	require.ErrorContains(t, txFnErr, "test error")

	dbItem1 = &model.Select{Id: createModel1.PKId}
	found, err = modelStore.Get(bgCtx, dbItem1)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, updatedName1, dbItem1.Name)
	require.Equal(t, flag1, dbItem1.Flag)

	dbItem2 = &model.Select{Id: createModel2.PKId}
	found, err = modelStore.Get(bgCtx, dbItem2)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, name2, dbItem2.Name)
	require.Equal(t, updatedFlag2, dbItem2.Flag)
}

func TestList(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	item := &model.Select{
		Name: "Test Model",
	}

	createModel := &model.Upsert{
		Name: &item.Name,
	}
	err = modelStore.Create(ctx, createModel)
	require.NoError(t, err)
	item.Id = createModel.PKId

	item2 := &model.Select{
		Name: "Test Model 2",
	}

	createModel = &model.Upsert{
		Name: &item2.Name,
	}
	err = modelStore.Create(ctx, createModel)
	require.NoError(t, err)
	item2.Id = createModel.PKId

	dbItems := make([]*model.Select, 0, 3)
	_, err = modelStore.List(ctx, mobone.ListParams{
		PageSize: 10,
		Sort:     []string{"id"},
	}, func(add bool) mobone.ListModelI {
		x := &model.Select{}
		if add {
			dbItems = append(dbItems, x)
		}
		return x
	})
	require.NoError(t, err)
	require.Len(t, dbItems, 2)
	dbItems[0].CreatedAt = time.Time{}
	dbItems[0].UpdatedAt = time.Time{}
	dbItems[1].CreatedAt = time.Time{}
	dbItems[1].UpdatedAt = time.Time{}
	require.Equal(t, item, dbItems[0])
	require.Equal(t, item2, dbItems[1])
}

func TestListWithInterceptor(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	item := &model.Select{
		Name: "Test Model",
	}

	createModel := &model.Upsert{
		Name: &item.Name,
	}
	err = modelStore.Create(ctx, createModel)
	require.NoError(t, err)
	item.Id = createModel.PKId

	dbItems := make([]*model.Select, 0, 3)
	_, err = modelStore.List(ctx, mobone.ListParams{
		PageSize: 10,
		Sort:     []string{"id"},
		Columns:  []string{"id"}, // for interceptor
	}, func(add bool) mobone.ListModelI {
		x := &model.Select{}
		if add {
			dbItems = append(dbItems, x)
		}
		return x
	})
	require.NoError(t, err)
	require.Len(t, dbItems, 1)
	require.Equal(t, item.Id, dbItems[0].Id)
}

func TestListWithOnlyCount(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	item := &model.Select{
		Name: "Test Model",
	}

	createModel := &model.Upsert{
		Name: &item.Name,
	}
	err = modelStore.Create(ctx, createModel)
	require.NoError(t, err)
	item.Id = createModel.PKId

	listCount, err := modelStore.List(ctx, mobone.ListParams{
		OnlyCount: true,
	}, func(add bool) mobone.ListModelI {
		return &model.Select{}
	})
	require.NoError(t, err)
	require.Equal(t, 1, int(listCount))
}

func TestDelete(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	item := &model.Select{
		Name: "Test Model",
	}

	createModel := &model.Upsert{
		Name: &item.Name,
	}
	err = modelStore.Create(ctx, createModel)
	require.NoError(t, err)
	item.Id = createModel.PKId

	deleteModel := &model.Upsert{PKId: item.Id}
	err = modelStore.Delete(ctx, deleteModel)
	require.NoError(t, err)

	listCount, err := modelStore.List(ctx, mobone.ListParams{
		OnlyCount: true,
	}, func(add bool) mobone.ListModelI {
		return &model.Select{}
	})
	require.NoError(t, err)
	require.Equal(t, 0, int(listCount))
}

func TestDeleteMany(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	name1 := "Delete 1"
	createModel1 := &model.Upsert{Name: &name1}
	err = modelStore.Create(ctx, createModel1)
	require.NoError(t, err)

	name2 := "Delete 2"
	createModel2 := &model.Upsert{Name: &name2}
	err = modelStore.Create(ctx, createModel2)
	require.NoError(t, err)

	err = modelStore.DeleteMany(ctx, []mobone.DeleteModelI{
		&model.Upsert{PKId: createModel1.PKId},
		&model.Upsert{PKId: createModel2.PKId},
	})
	require.NoError(t, err)

	listCount, err := modelStore.List(ctx, mobone.ListParams{
		OnlyCount: true,
	}, func(add bool) mobone.ListModelI {
		return &model.Select{}
	})
	require.NoError(t, err)
	require.Equal(t, 0, int(listCount))
}

func TestJsonMerge(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	ctx := context.Background()

	modelStore := mobone.ModelStore{
		Con:       dbCon.pool,
		QB:        queryBuilder,
		TableName: tableName,
	}

	item := &model.Select{
		Name: "Name",
		Contact: model.Contact{
			Email: "test@example.com",
		},
	}

	createModel := &model.Upsert{
		Name: &item.Name,
		Contact: &model.ContactEdit{
			Email: &item.Contact.Email,
		},
	}
	err = modelStore.Create(ctx, createModel)
	require.NoError(t, err)
	item.Id = createModel.PKId

	dbItem := &model.Select{Id: item.Id}
	_, err = modelStore.Get(ctx, dbItem)
	require.NoError(t, err)
	dbItem.CreatedAt = time.Time{}
	dbItem.UpdatedAt = time.Time{}
	require.Equal(t, item, dbItem)

	item.Contact.Phone = "123456789"

	err = modelStore.Update(ctx, &model.Upsert{
		PKId: item.Id,
		Contact: &model.ContactEdit{
			Phone: &item.Contact.Phone,
		},
	})
	require.NoError(t, err)

	dbItem = &model.Select{Id: item.Id}
	_, err = modelStore.Get(ctx, dbItem)
	require.NoError(t, err)
	dbItem.CreatedAt = time.Time{}
	dbItem.UpdatedAt = time.Time{}
	require.Equal(t, item, dbItem)

	item.Contact.Email = "changed@example.com"

	err = modelStore.Update(ctx, &model.Upsert{
		PKId: item.Id,
		Contact: &model.ContactEdit{
			Email: &item.Contact.Email,
		},
	})
	require.NoError(t, err)

	dbItem = &model.Select{Id: item.Id}
	_, err = modelStore.Get(ctx, dbItem)
	require.NoError(t, err)
	dbItem.CreatedAt = time.Time{}
	dbItem.UpdatedAt = time.Time{}
	require.Equal(t, item, dbItem)

	item.Contact.Email = ""

	err = modelStore.Update(ctx, &model.Upsert{
		PKId: item.Id,
		Contact: &model.ContactEdit{
			Email: &item.Contact.Email,
		},
	})
	require.NoError(t, err)

	dbItem = &model.Select{Id: item.Id}
	_, err = modelStore.Get(ctx, dbItem)
	require.NoError(t, err)
	dbItem.CreatedAt = time.Time{}
	dbItem.UpdatedAt = time.Time{}
	require.Equal(t, item, dbItem)
}

func TestTransaction(t *testing.T) {
	_, err := dbCon.pool.Exec(context.Background(), "truncate table "+tableName+" RESTART IDENTITY")
	require.NoError(t, err)

	bgCtx := context.Background()

	txM := mobone.NewTransactionManager(dbCon.pool)

	modelStore := mobone.ModelStore{
		Con:                dbCon.pool,
		TransactionManager: txM,
		QB:                 queryBuilder,
		TableName:          tableName,
	}

	item := &model.Select{
		Name: "Test Model",
		Flag: true,
		Contact: model.Contact{
			Phone: "123456789",
			Email: "test@example.com",
		},
	}

	createModel := &model.Upsert{
		Name: &item.Name,
		Flag: &item.Flag,
		Contact: &model.ContactEdit{
			Phone: &item.Contact.Phone,
			Email: &item.Contact.Email,
		},
	}
	err = modelStore.Create(bgCtx, createModel)
	require.NoError(t, err)
	item.Id = createModel.PKId

	item.UpdatedAt = time.Now().Add(-time.Hour)
	item.Name = "Test Model changed"
	item.Flag = false
	item.Contact.Phone = "987654321"
	item.Contact.Email = "changed@example.com"

	updateModel := &model.Upsert{
		PKId:      item.Id,
		UpdatedAt: &item.UpdatedAt,
		Name:      &item.Name,
		Flag:      &item.Flag,
		Contact: &model.ContactEdit{
			Phone: &item.Contact.Phone,
			Email: &item.Contact.Email,
		},
	}
	txFnErr := txM.TxFn(bgCtx, func(ctx context.Context) error {
		return modelStore.Update(ctx, updateModel)
	})
	require.NoError(t, txFnErr)
	require.Greater(t, updateModel.PKId, 0)
	item.Id = updateModel.PKId

	dbItem := &model.Select{Id: item.Id}
	found, err := modelStore.Get(bgCtx, dbItem)
	require.NoError(t, err)
	require.True(t, found)
	require.WithinDuration(t, time.Now(), dbItem.CreatedAt, 30*time.Millisecond)
	require.WithinDuration(t, item.UpdatedAt, dbItem.UpdatedAt, 30*time.Millisecond)
	dbItem.CreatedAt = time.Time{}
	dbItem.UpdatedAt = item.UpdatedAt
	require.Equal(t, item, dbItem)

	newName := "Test Model changed again"
	updateModel = &model.Upsert{
		PKId: item.Id,
		Name: &newName,
	}
	txFnErr = txM.TxFn(bgCtx, func(ctx context.Context) error {
		err = modelStore.Update(ctx, updateModel)
		if err != nil {
			return err
		}
		return fmt.Errorf("test error")
	})
	require.NotNil(t, txFnErr, "TxFn should return error")
	require.ErrorContains(t, txFnErr, "test error")

	dbItem = &model.Select{Id: item.Id}
	found, err = modelStore.Get(bgCtx, dbItem)
	require.NoError(t, err)
	require.True(t, found)
	dbItem.CreatedAt = time.Time{}
	dbItem.UpdatedAt = item.UpdatedAt
	require.Equal(t, item, dbItem)

	var noActive bool
	err = modelStore.Con.QueryRow(bgCtx, `
        select count(*) = 0 as no_active_tx
        from pg_stat_activity
        where state in ('idle in transaction', 'idle in transaction (aborted)')
          and pid <> pg_backend_pid();
    `).Scan(&noActive)
	require.NoError(t, err)
	require.True(t, noActive)
}
