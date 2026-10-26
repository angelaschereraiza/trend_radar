package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/angelaschereraiza/trend_radar/backend/internal/config"
	"github.com/angelaschereraiza/trend_radar/backend/internal/marketdata"
	"github.com/angelaschereraiza/trend_radar/backend/internal/repository"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--fixture" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/collector --fixture")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL())
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		slog.Error("connect to database", "error", err)
		os.Exit(1)
	}
	batch := marketdata.NewFixtureProvider().Batch()
	report, err := repository.NewMarketData(db).Import(ctx, batch, "Deterministic local fixture dataset", true)
	if err != nil {
		slog.Error("import fixture data", "error", err)
		os.Exit(1)
	}
	slog.Info("fixture data imported",
		"snapshot_id", report.SnapshotID,
		"inserted_rows", report.Inserted,
		"unchanged_rows", report.Unchanged,
		"survivorship_biased", true,
	)
}
