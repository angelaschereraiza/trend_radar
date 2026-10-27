package main

import (
	"context"
	"database/sql"
	"encoding/json"
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
	mode := ""
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if !validCollectorArgs(mode, os.Args[2:]) {
		printUsage()
		os.Exit(2)
	}
	if mode == "--eodhd-exchanges" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		exchanges, err := marketdata.NewEODHDProvider(os.Getenv("EODHD_API_TOKEN")).FetchExchanges(ctx)
		if err != nil {
			slog.Error("fetch EODHD exchanges", "error", err)
			os.Exit(1)
		}
		if err := json.NewEncoder(os.Stdout).Encode(exchanges); err != nil {
			slog.Error("write EODHD exchanges", "error", err)
			os.Exit(1)
		}
		return
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
	survivorshipBiased := true
	switch mode {
	case "--yahoo":
		to := time.Now().UTC()
		from := to.AddDate(-10, 0, 0)
		batch, err = marketdata.NewYahooProvider().FetchBatch(ctx, from, to)
		description = "Yahoo Finance daily history for NESN, NOVN, ROG, SPY and USD/CHF; limited current universe"
	case "--fixture":
		batch = marketdata.NewFixtureProvider().Batch()
	case "--eodhd-catalog":
		exchange := os.Args[2]
		symbols, fetchErr := marketdata.NewEODHDProvider(os.Getenv("EODHD_API_TOKEN")).FetchCatalog(ctx, exchange)
		err = fetchErr
		batch = marketdata.ImportBatch{Symbols: symbols}
		description = "EODHD active common-stock catalog for " + exchange
		survivorshipBiased = false
	case "--eodhd-history":
		exchange, ticker := os.Args[2], os.Args[3]
		from, to, dateErr := eodhdHistoryRange(os.Args[4:])
		if dateErr != nil {
			slog.Error("invalid EODHD history range", "error", dateErr)
			os.Exit(2)
		}
		batch, err = marketdata.NewEODHDProvider(os.Getenv("EODHD_API_TOKEN")).FetchHistory(ctx, exchange, ticker, from, to)
		description = fmt.Sprintf("EODHD daily history for %s.%s", ticker, exchange)
	default:
		printUsage()
		os.Exit(2)
	}
	if err != nil {
		slog.Error("fetch market data", "provider", mode, "error", err)
		os.Exit(1)
	}
	report, err := repository.NewMarketData(db).Import(ctx, batch, description, survivorshipBiased)
	if err != nil {
		slog.Error("import market data", "provider", mode, "error", err)
		os.Exit(1)
	}
	slog.Info("market data imported",
		"provider", mode,
		"symbols", len(batch.Symbols),
		"snapshot_id", report.SnapshotID,
		"inserted_rows", report.Inserted,
		"unchanged_rows", report.Unchanged,
		"survivorship_biased", survivorshipBiased,
	)
}

func validCollectorArgs(mode string, args []string) bool {
	switch mode {
	case "--fixture", "--yahoo", "--eodhd-exchanges":
		return len(args) == 0
	case "--eodhd-catalog":
		return len(args) == 1
	case "--eodhd-history":
		return len(args) == 2 || len(args) == 4
	default:
		return false
	}
}

func eodhdHistoryRange(args []string) (time.Time, time.Time, error) {
	to := time.Now().UTC()
	from := to.AddDate(-1, 0, 0)
	if len(args) == 0 {
		return from, to, nil
	}
	from, err := time.Parse("2006-01-02", args[0])
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse start date: %w", err)
	}
	to, err = time.Parse("2006-01-02", args[1])
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse end date: %w", err)
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("start date must be on or before end date")
	}
	return from, to, nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  go run ./cmd/collector --fixture")
	fmt.Fprintln(os.Stderr, "  go run ./cmd/collector --yahoo")
	fmt.Fprintln(os.Stderr, "  go run ./cmd/collector --eodhd-exchanges")
	fmt.Fprintln(os.Stderr, "  go run ./cmd/collector --eodhd-catalog <exchange>")
	fmt.Fprintln(os.Stderr, "  go run ./cmd/collector --eodhd-history <exchange> <ticker> [from YYYY-MM-DD to YYYY-MM-DD]")
}
