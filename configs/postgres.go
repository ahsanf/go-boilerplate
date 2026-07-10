package configs

import (
	"context"

	"go-boilerplate/internal/utils"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

var PostgresPool *pgxpool.Pool

func ConnectPostgres(ctx context.Context) {
	if Cfg.PostgresURL == "" {
		utils.Logger.Fatal("POSTGRES_URL is not set")
	}

	pool, err := pgxpool.New(ctx, Cfg.PostgresURL)
	if err != nil {
		utils.Logger.Fatal("failed to create Postgres pool", zap.Error(err))
	}

	if err := pool.Ping(ctx); err != nil {
		utils.Logger.Fatal("failed to ping Postgres", zap.Error(err))
	}

	PostgresPool = pool
	utils.Logger.Info("connected to Postgres")
}
