// Package logger - структурные логи приложения на log/slog
package logger

import (
	"log/slog"
	"os"
)

// Setup - логгер по умолчанию для всего приложения, в том числе для стандартного log:
// JSON по записи на строку с уровнем info, такие логи фильтруются по полям
func Setup() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
}

// Fatal - ошибка, после которой работать нельзя (обычно при запуске): пишет её и завершает процесс
func Fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
