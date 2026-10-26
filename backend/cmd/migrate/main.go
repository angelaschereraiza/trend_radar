package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/angelaschereraiza/trend_radar/backend/internal/config"
	"github.com/angelaschereraiza/trend_radar/backend/migrations"
)

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "up" && os.Args[1] != "down") {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/migrate {up|down [steps]}")
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		slog.Error("connect to database", "error", err)
		os.Exit(1)
	}
	if os.Args[1] == "up" {
		err = migrations.Up(ctx, db)
	} else {
		steps := 1
		if len(os.Args) > 2 {
			steps, err = strconv.Atoi(os.Args[2])
			if err != nil {
				slog.Error("invalid migration steps", "error", err)
				os.Exit(2)
			}
		}
		err = migrations.Down(ctx, db, steps)
	}
	if err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
}
