package tests

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Con struct {
	pool *pgxpool.Pool
}

func NewCon(dbName string) (*Con, error) {
	connString := ""
	if dbName != "" {
		connString = "dbname=" + dbName
	}

	pgxpoolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.ParseConfig: %w", err)
	}

	pgxpoolConfig.MaxConnIdleTime = time.Minute
	pgxpoolConfig.MaxConns = 10
	pgxpoolConfig.MinConns = 1
	pgxpoolConfig.HealthCheckPeriod = time.Minute

	con, err := pgxpool.NewWithConfig(context.Background(), pgxpoolConfig)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.NewWithConfig: %w", err)
	}

	return &Con{
		pool: con,
	}, nil
}

func (c *Con) Close() {
	c.pool.Close()
}
