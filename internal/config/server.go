package config

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// addr - только loopback: на проде запросы приходят через Caddy на том же сервере, напрямую порт недоступен
const addr string = `127.0.0.1:8000`

// NewServer - собирает http-сервер вокруг handler
func NewServer(handler http.Handler) *http.Server {
	log := slog.Default()

	return &http.Server{
		Addr:              addr,
		Handler:           withMiddleware(handler, log),
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
		ReadHeaderTimeout: time.Second * 5,
		// фото карты с камеры - до 10 МБ, по мобильной сети в подвале магазина оно едет не за секунды
		ReadTimeout: time.Minute,
		// распознавание QR на большом фото занимает до пары секунд
		WriteTimeout: time.Second * 30,
		IdleTimeout:  time.Minute,
	}
}

// withMiddleware - обёртки всех запросов, изнутри наружу
func withMiddleware(handler http.Handler, log *slog.Logger) http.Handler {
	// запросы с чужих сайтов, кроме GET, HEAD и OPTIONS, получают 403: чужая страница не отправит форму
	// от имени вошедшего пользователя (CSRF). Проверка по заголовкам Sec-Fetch-Site и Origin дополняет
	// SameSite у куки входа и стоит внутри журнала, так что отказы тоже в него попадают
	handler = http.NewCrossOriginProtection().Handler(handler)
	// recoverer внутри журнала: ответ 500 после паники тоже попадает в журнал
	handler = recoverer(log)(handler)
	handler = requestLogger(log)(handler)

	// id запроса нужен журналу: по нему ошибка обработчика находится рядом со строкой самого запроса
	return middleware.RequestID(handler)
}
