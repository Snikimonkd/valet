package main

import (
	"context"
	"log/slog"

	"github.com/benbjohnson/clock"

	"valet/internal/app/delivery"
	"valet/internal/app/repository"
	"valet/internal/app/usecase"
	"valet/internal/config"
	"valet/internal/pkg/logger"
)

func main() {
	logger.Setup()
	slog.Info("runtime start")

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("can't read config", err)
	}

	ctx := context.Background()

	db, err := config.ConnectToPostgres(ctx, cfg.PgDSN)
	if err != nil {
		logger.Fatal("can't start postgres", err)
	}
	slog.Info("db ready")

	repository := repository.NewRepository(db)
	usecase := usecase.NewUsecase(repository, clock.New())
	delivery, err := delivery.New(usecase, cfg.Domain)
	if err != nil {
		logger.Fatal("can't init delivery ui", err)
	}

	server := config.NewServer(delivery)

	slog.Info("starting server", "addr", server.Addr)
	err = server.ListenAndServe()
	if err != nil {
		logger.Fatal("can't start server", err)
	}
}
