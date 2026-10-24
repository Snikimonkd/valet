package config

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// requestLogger - пишет каждый запрос одной структурной записью: метод, путь, статус, время обработки и id запроса.
// Строку запроса не пишет: в ней ничего полезного для журнала, а мусор копится
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				// обработчик ничего не записал: net/http отдаст 200
				status = http.StatusOK
			}

			log.LogAttrs(r.Context(), slog.LevelInfo, "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", middleware.GetReqID(r.Context())),
			)
		})
	}
}

// recoverer - паника в обработчике не роняет сервер: пишет её со стеком одной записью и отвечает 500
func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer recoverPanic(r.Context(), log, w)

			next.ServeHTTP(w, r)
		})
	}
}

// recoverPanic - вызывается только через defer: recover срабатывает, лишь когда его вызывает сама отложенная функция
func recoverPanic(ctx context.Context, log *slog.Logger, w http.ResponseWriter) {
	rec := recover()
	if rec == nil {
		return
	}

	// так net/http намеренно обрывает ответ, это не ошибка
	err, _ := rec.(error)
	if errors.Is(err, http.ErrAbortHandler) {
		panic(rec)
	}

	log.LogAttrs(ctx, slog.LevelError, "panic",
		slog.Any("panic", rec),
		slog.String("stack", string(debug.Stack())),
		slog.String("request_id", middleware.GetReqID(ctx)),
	)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
