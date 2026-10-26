package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/angelaschereraiza/trend_radar/backend/internal/api"
	"github.com/angelaschereraiza/trend_radar/backend/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(1)
	}

	level := new(slog.LevelVar)
	switch cfg.LogLevel {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	db, err := sql.Open("pgx", cfg.DatabaseURL())
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := api.RecoverInterruptedBacktests(recoveryCtx, db); err != nil {
		recoveryCancel()
		logger.Error("recover interrupted backtests", "error", err)
		os.Exit(1)
	}
	recoveryCancel()

	application := api.NewHandler(db)
	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.APIHost, fmt.Sprint(cfg.APIPort)),
		Handler:           application,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown API server", "error", err)
		}
		if err := application.Shutdown(shutdownCtx); err != nil {
			logger.Error("wait for backtests to stop", "error", err)
		}
	}()

	logger.Info("starting API server", "address", server.Addr, "environment", cfg.Environment)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("API server failed", "error", err)
		os.Exit(1)
	}
}
