package delivery

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"valet/internal/app/usecase"
)

// sessionCookie - кука с токеном сессии
const sessionCookie string = `valet-session`

// userKey - ключ id пользователя в контексте запроса, его кладёт requireUser
type userKey struct{}

// userID - id вошедшего пользователя. Есть только в обработчиках внутри requireUser
func userID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(userKey{}).(uuid.UUID)

	return id
}

func hasUser(ctx context.Context) bool {
	return userID(ctx) != uuid.Nil
}

// requireUser - пускает дальше только с живой сессией и кладёт id пользователя в контекст.
// Без сессии - на страницу входа. Продлённую сессию продлевает и в куке, иначе браузер выкинет её раньше базы
func (wb *delivery) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			redirect(w, r, "/login")
			return
		}

		session, err := wb.usecase.Authenticate(r.Context(), cookie.Value)
		if errors.Is(err, usecase.ErrUnauthorized) {
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			wb.serverError(w, r, err)
			return
		}

		if session.Renewed {
			http.SetCookie(w, wb.newSessionCookie(session.Token, session.ExpiresAt))
		}

		ctx := context.WithValue(r.Context(), userKey{}, session.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type loginView struct {
	Login string
	Error string
}

func (wb *delivery) loginPage(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookie)
	if err == nil {
		_, err = wb.usecase.Authenticate(r.Context(), cookie.Value)
		if err == nil {
			redirect(w, r, "/")
			return
		}
	}

	wb.renderPage(w, r, http.StatusOK, pageLogin, loginView{})
}

func (wb *delivery) login(w http.ResponseWriter, r *http.Request) {
	view := loginView{
		Login: strings.TrimSpace(r.PostFormValue("login")),
	}
	password := r.PostFormValue("password")

	if view.Login == "" || password == "" {
		view.Error = "Введите логин и пароль"
		wb.renderPage(w, r, http.StatusUnprocessableEntity, pageLogin, view)
		return
	}

	session, err := wb.usecase.Login(r.Context(), view.Login, password)
	if errors.Is(err, usecase.ErrWrongPassword) {
		view.Error = "Неверный логин или пароль"
		wb.renderPage(w, r, http.StatusUnprocessableEntity, pageLogin, view)
		return
	}
	if err != nil {
		wb.serverError(w, r, err)
		return
	}

	http.SetCookie(w, wb.newSessionCookie(session.Token, session.ExpiresAt))
	redirect(w, r, "/")
}

func (wb *delivery) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookie)
	if err == nil {
		err = wb.usecase.Logout(r.Context(), cookie.Value)
		if err != nil {
			wb.serverError(w, r, err)
			return
		}
	}

	// браузер удалит куку, только если имя, путь и домен совпадают с кукой входа.
	// Офлайн-копии карт стирает service worker, когда видит этот запрос, см. static/sw.js.
	// Clear-Site-Data тут не годится: Chrome держит навигацию, пока чистит весь кэш сайта,
	// и заодно выкидывает статику, которой незачем пропадать
	http.SetCookie(w, wb.newSessionCookie("", time.Unix(0, 0)))
	redirect(w, r, "/login")
}

// newSessionCookie - кука сессии: только по https, из js не видна
func (wb *delivery) newSessionCookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Expires:  expires,
		Path:     "/",
		Domain:   wb.domain,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}
