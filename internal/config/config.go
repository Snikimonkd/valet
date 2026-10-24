package config

import (
	"errors"
	"os"
)

// переменные окружения с настройками
const (
	envDomain string = "DOMAIN"
	envPgDSN  string = "PG_DSN"
)

// Config - настройки сервиса из переменных окружения
type Config struct {
	// Domain - домен куки входа: localhost локально, свой домен на проде
	Domain string
	// PgDSN - строка подключения к postgres
	PgDSN string
}

// Load - читает настройки из окружения. DOMAIN и PG_DSN обязательны
func Load() (Config, error) {
	cfg := Config{
		Domain: os.Getenv(envDomain),
		PgDSN:  os.Getenv(envPgDSN),
	}

	if cfg.Domain == "" {
		return Config{}, errors.New("empty env variable " + envDomain)
	}
	if cfg.PgDSN == "" {
		return Config{}, errors.New("empty env variable " + envPgDSN)
	}

	return cfg, nil
}
