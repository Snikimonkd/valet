package delivery

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
)

//go:embed templates static
var files embed.FS

// Usecase - бизнес-логика, которую использует веб-интерфейс
//
//go:generate go tool mockgen -typed -destination=mock/usecase_mock.go -package=mock . Usecase
type Usecase interface {
	SessionUsecase
	CardsUsecase
}

// SessionUsecase - регистрация, вход, проверка сессии и выход
type SessionUsecase interface {
	Register(ctx context.Context, in usecase.RegisterInput) (usecase.Session, error)
	Login(ctx context.Context, login, password string) (usecase.Session, error)
	Authenticate(ctx context.Context, token string) (usecase.Session, error)
	Logout(ctx context.Context, token string) error
}

// CardsUsecase - карты пользователя и распознавание QR-кода с картинки
type CardsUsecase interface {
	ListCards(ctx context.Context, userID uuid.UUID) ([]model.Cards, error)
	GetCard(ctx context.Context, userID uuid.UUID, cardID int64) (model.Cards, error)
	OpenCard(ctx context.Context, userID uuid.UUID, cardID int64) (model.Cards, error)
	CreateCard(ctx context.Context, userID uuid.UUID, in usecase.CardInput) (int64, error)
	UpdateCard(ctx context.Context, userID uuid.UUID, cardID int64, in usecase.CardInput) error
	DeleteCard(ctx context.Context, userID uuid.UUID, cardID int64) error
	CardQR(ctx context.Context, userID uuid.UUID, cardID int64) ([]byte, error)
	ScanQR(ctx context.Context, r io.Reader) (string, error)
}

// delivery - html-интерфейс сервиса: http.Handler со всеми страницами и статикой
type delivery struct {
	usecase Usecase
	// domain - домен куки входа
	domain string
	pages  map[page]*template.Template
	assets assets
	router http.Handler
}

// New - конструктор для delivery. domain - домен куки входа: localhost локально, свой домен на проде
func New(uc Usecase, domain string) (*delivery, error) {
	static, err := fs.Sub(files, "static")
	if err != nil {
		return nil, fmt.Errorf("can't open static files: %w", err)
	}

	assets, err := hashAssets(static)
	if err != nil {
		return nil, err
	}

	pages, err := parsePages(files, assets)
	if err != nil {
		return nil, err
	}

	wb := &delivery{
		usecase: uc,
		domain:  domain,
		pages:   pages,
		assets:  assets,
	}
	wb.router = wb.routes(static)

	return wb, nil
}

func (wb *delivery) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wb.router.ServeHTTP(w, r)
}

// routes - маршруты html-страниц и статики
func (wb *delivery) routes(static fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(securityHeaders)

	r.Handle("/static/*", http.StripPrefix("/static/", wb.assets.handler(static)))
	// service worker управляет только адресами не выше своего, поэтому живёт в корне, а не в /static/
	r.Get("/sw.js", wb.serviceWorker(static))
	r.Get("/manifest.webmanifest", wb.manifest(static))

	r.Get("/login", wb.loginPage)
	r.Post("/login", wb.login)
	r.Get("/register", wb.registerPage)
	r.Post("/register", wb.register)
	r.Post("/logout", wb.logout)

	r.Group(func(r chi.Router) {
		r.Use(wb.requireUser)
		r.Get("/", wb.cardsPage)
		r.Get("/ping", wb.ping)
		r.Get("/cards/new", wb.newCardPage)
		r.Get("/cards/new/manual", wb.manualCardPage)
		r.Post("/cards/scan", wb.scanCard)
		r.Post("/cards", wb.createCard)
		r.Get("/cards/{id}", wb.cardPage)
		r.Get("/cards/{id}/qr.png", wb.cardQR)
		r.Get("/cards/{id}/edit", wb.editCardPage)
		r.Post("/cards/{id}", wb.updateCard)
		r.Get("/cards/{id}/delete", wb.deleteCardPage)
		r.Post("/cards/{id}/delete", wb.deleteCard)
	})

	return r
}

// securityHeaders - заголовки, которые браузер применяет к каждой странице.
// CSP пускает скрипты, стили и картинки только с нашего домена: внедрённый в название карты скрипт не выполнится,
// даже если где-то забудется экранирование. data: - для узоров и иконок, вшитых прямо в CSS и SVG
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; "+
			"object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")

		next.ServeHTTP(w, r)
	})
}

func parsePages(fsys fs.FS, assets assets) (map[page]*template.Template, error) {
	base, err := template.New("").Funcs(funcs(assets)).ParseFS(fsys, "templates/layout.html", "templates/partials.html")
	if err != nil {
		return nil, fmt.Errorf("can't parse base templates: %w", err)
	}

	pages := make(map[page]*template.Template, len(allPages))
	for _, p := range allPages {
		tmpl, cerr := base.Clone()
		if cerr != nil {
			return nil, fmt.Errorf("can't clone base templates: %w", cerr)
		}

		tmpl, cerr = tmpl.ParseFS(fsys, p.templatePath())
		if cerr != nil {
			return nil, fmt.Errorf("can't parse page %s: %w", p, cerr)
		}

		pages[p] = tmpl
	}

	// шаблон без константы в allPages никогда не отрендерится - скорее всего, его забыли добавить
	names, err := fs.Glob(fsys, "templates/pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("can't list page templates: %w", err)
	}
	for _, name := range names {
		if _, ok := pages[page(strings.TrimSuffix(path.Base(name), ".html"))]; !ok {
			return nil, fmt.Errorf("template %s has no page constant in allPages", name)
		}
	}

	return pages, nil
}

func funcs(assets assets) template.FuncMap {
	return template.FuncMap{
		// /static/app.css?v=<хэш>, см. assets.handler
		"asset": assets.url,
	}
}

// layoutData - общие данные для всех страниц
type layoutData struct {
	Title pageTitle
	// Name - имя страницы: по нему app.js понимает, что делать на этой странице
	Name page
	// LoggedIn - показывать ли кнопку выхода
	LoggedIn bool
	Page     any
}

// renderPage - рендерит страницу целиком: общий каркас с заголовком страницы и её данные view
func (wb *delivery) renderPage(w http.ResponseWriter, r *http.Request, status int, p page, view any) {
	data := layoutData{
		Title:    p.title(),
		Name:     p,
		LoggedIn: hasUser(r.Context()),
		Page:     view,
	}

	wb.execute(w, r, status, p, "layout", data)
}

// renderContent - на htmx-запрос только содержимое страницы без каркаса, на обычный - страница целиком
func (wb *delivery) renderContent(w http.ResponseWriter, r *http.Request, status int, p page, view any) {
	if !isHTMX(r) {
		wb.renderPage(w, r, status, p, view)
		return
	}

	// content ждёт те же данные, что и в каркасе: страница читает свои поля из .Page
	wb.execute(w, r, status, p, "content", layoutData{Title: p.title(), Name: p, LoggedIn: true, Page: view})
}

func (wb *delivery) execute(w http.ResponseWriter, r *http.Request, status int, p page, block string, data any) {
	tmpl, ok := wb.pages[p]
	if !ok {
		slog.ErrorContext(r.Context(), "unknown page template", "page", string(p), "request_id", requestID(r))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	err := tmpl.ExecuteTemplate(&buf, block, data)
	if err != nil {
		slog.ErrorContext(r.Context(), "can't render template",
			"page", string(p), "block", block, "err", err, "request_id", requestID(r))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// страницы с картами персональные: ни в кэше браузера, ни в кэшах по дороге их быть не должно,
	// и после выхода «назад» не покажет список карт. Офлайн-копии хранит service worker
	// в Cache API, а он на этот заголовок не смотрит
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// requestID - id запроса из журнала запросов: по нему ошибка находится рядом со строкой самого запроса
func requestID(r *http.Request) string {
	return middleware.GetReqID(r.Context())
}

func (wb *delivery) serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "request failed", "err", err, "request_id", requestID(r))
	http.Error(w, "Что-то пошло не так. Попробуйте ещё раз через минуту.", http.StatusInternalServerError)
}

// isHTMX - запрос пришёл от htmx, а не обычной навигацией браузера
func isHTMX(r *http.Request) bool {
	return r.Header.Get("Hx-Request") == "true"
}

// redirect - перенаправление, которое понимают и браузер, и htmx
func redirect(w http.ResponseWriter, r *http.Request, url string) {
	if isHTMX(r) {
		w.Header().Set("Hx-Redirect", url)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	http.Redirect(w, r, url, http.StatusSeeOther)
}
