package mobone

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type transactionCtxKeyT int8

const transactionCtxKey = transactionCtxKeyT(1)

type TransactionManager struct {
	con *pgxpool.Pool
}

func NewTransactionManager(con *pgxpool.Pool) *TransactionManager {
	return &TransactionManager{
		con: con,
	}
}

func (s *TransactionManager) getContextTransaction(ctx context.Context) pgx.Tx {
	contextV := ctx.Value(transactionCtxKey)
	if contextV == nil {
		return nil
	}

	if tx, ok := contextV.(pgx.Tx); ok {
		return tx
	}

	return nil
}

func (s *TransactionManager) contextWithTransaction(ctx context.Context) (context.Context, pgx.Tx, error) {
	if tx := s.getContextTransaction(ctx); tx != nil {
		return ctx, tx, nil
	}

	tx, err := s.con.Begin(ctx)
	if err != nil {
		return ctx, nil, fmt.Errorf("unable to begin transaction: %w", err)
	}

	return context.WithValue(ctx, transactionCtxKey, tx), tx, nil
}

func (s *TransactionManager) GetConnection(ctx context.Context) ConnectionI {
	if tx := s.getContextTransaction(ctx); tx != nil {
		return tx
	}

	return s.con
}

func (s *TransactionManager) TxFn(ctx context.Context, f func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ctxWithTx, tx, err := s.contextWithTransaction(ctx)
	if err != nil {
		return err
	}

	// defer rollback
	defer func() { _ = tx.Rollback(ctx) }()

	// run transaction function
	err = f(ctxWithTx)
	if err != nil {
		return fmt.Errorf("transaction function: %w", err)
	}

	// commit
	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("transaction commit: %w", err)
	}

	return nil
}
