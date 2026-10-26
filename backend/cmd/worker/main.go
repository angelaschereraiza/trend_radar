package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "time/tzdata"

	"github.com/angelaschereraiza/trend_radar/backend/internal/config"
	"github.com/angelaschereraiza/trend_radar/backend/internal/repository"
	"github.com/angelaschereraiza/trend_radar/backend/internal/scheduler"
	"github.com/angelaschereraiza/trend_radar/backend/internal/worker"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "--rankings" && os.Args[1] != "--schedule") {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/worker --rankings | --schedule")
		os.Exit(2)
	}
	mode := os.Args[1]
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
	if mode == "--schedule" {
		scheduled, err := scheduler.New(db, cfg, slog.Default())
		if err != nil {
			slog.Error("create scheduler", "error", err)
			os.Exit(1)
		}
		if err := scheduled.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("scheduler stopped", "error", err)
			os.Exit(1)
		}
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		slog.Error("connect to database", "error", err)
		os.Exit(1)
	}
	repository := repository.NewMarketData(db)
	history, err := repository.LoadLatestPriceHistory(ctx)
	if err != nil {
		slog.Error("load market history", "error", err)
		os.Exit(1)
	}
	fxRates, err := repository.LoadLatestFXRates(ctx, "USD", "CHF")
	if err != nil {
		slog.Error("load FX rates", "error", err)
		os.Exit(1)
	}
	run, err := worker.CalculateLatestRanking(history, fxRates, cfg.MaxCapitalCHF)
	if err != nil {
		slog.Error("calculate ranking", "error", err)
		os.Exit(1)
	}
	if err := repository.SaveRanking(ctx, "momentum_v1", "1", run.Date, run.Entries, run.Correlations); err != nil {
		slog.Error("persist ranking", "error", err)
		os.Exit(1)
	}
	slog.Info("ranking persisted",
		"calculation_date", run.Date.Format("2006-01-02"),
		"ranked_stocks", len(run.Entries),
		"excluded_stocks", len(run.Excluded),
	)
	for symbol, reason := range run.Excluded {
		slog.Warn("stock excluded from ranking", "symbol", symbol, "reason", reason)
	}
}
