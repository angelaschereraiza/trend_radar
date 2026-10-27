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
	if len(os.Args) != 2 || (os.Args[1] != "--fixture" && os.Args[1] != "--yahoo") {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/collector --fixture | --yahoo")
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
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		slog.Error("connect to database", "error", err)
		os.Exit(1)
	}
	var batch marketdata.ImportBatch
	description := "Deterministic local fixture dataset"
	if os.Args[1] == "--yahoo" {
		to := time.Now().UTC()
		from := to.AddDate(-10, 0, 0)
		batch, err = marketdata.NewYahooProvider().FetchBatch(ctx, from, to)
		description = "Yahoo Finance daily history for NESN, SPY and USD/CHF; limited current universe"
	} else {
		batch = marketdata.NewFixtureProvider().Batch()
	}
	if err != nil {
		slog.Error("fetch market data", "provider", os.Args[1], "error", err)
		os.Exit(1)
	}
	report, err := repository.NewMarketData(db).Import(ctx, batch, description, true)
	if err != nil {
		slog.Error("import market data", "provider", os.Args[1], "error", err)
		os.Exit(1)
	}
	slog.Info("market data imported",
		"provider", os.Args[1],
		"snapshot_id", report.SnapshotID,
		"inserted_rows", report.Inserted,
		"unchanged_rows", report.Unchanged,
		"survivorship_biased", true,
	)
}
