package delivery

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"valet/internal/app/delivery/mock"
	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
)

const (
	testToken string = "session-token"
	// testDomain - домен куки входа в тестах
	testDomain string = "valet.example"
)

var testNow = time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC)

// testUserID - пользователь, от имени которого идут запросы с testToken
var testUserID = uuid.MustParse("019a1d6e-0000-7000-8000-000000000007")

// mockUsecase - мок usecase без ожиданий: вызов, которого тест не задал, роняет тест
func mockUsecase(t *testing.T) *mock.MockUsecase {
	t.Helper()

	return mock.NewMockUsecase(gomock.NewController(t))
}

func newTestDelivery(t *testing.T, uc Usecase) *delivery {
	t.Helper()

	wb, err := New(uc, testDomain)
	if err != nil {
		t.Fatal(err)
	}

	return wb
}

// expectSession - запрос с testToken пройдёт requireUser как testUserID
func expectSession(uc *mock.MockUsecase) {
	uc.EXPECT().Authenticate(gomock.Any(), testToken).
		Return(usecase.Session{Token: testToken, UserID: testUserID, ExpiresAt: testNow.Add(time.Hour)}, nil).
		AnyTimes()
}

// do - запрос к delivery. С авторизацией - кука testToken. Тело формы - form, если не nil
func do(
	t *testing.T, wb http.Handler, method, target string, form url.Values, authorized bool,
) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequestWithContext(t.Context(), method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if authorized {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: testToken})
	}

	rec := httptest.NewRecorder()
	wb.ServeHTTP(rec, req)

	return rec
}

func testCard(id int64, name string) model.Cards {
	return model.Cards{
		ID: id, UserID: testUserID, Name: name, Content: "2775012345678", Format: "qr",
		CreatedAt: testNow, UpdatedAt: testNow, LastUsedAt: testNow,
	}
}

func TestRequireUser(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("db is down")
	tests := []struct {
		name       string
		cookie     bool
		authErr    error
		wantStatus int
	}{
		{name: "no cookie", wantStatus: http.StatusSeeOther},
		{name: "expired session", cookie: true, authErr: usecase.ErrUnauthorized, wantStatus: http.StatusSeeOther},
		{name: "db error", cookie: true, authErr: dbErr, wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := mockUsecase(t)
			if tt.cookie {
				uc.EXPECT().Authenticate(gomock.Any(), testToken).Return(usecase.Session{}, tt.authErr)
			}
			wb := newTestDelivery(t, uc)

			rec := do(t, wb, http.MethodGet, "/", nil, tt.cookie)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusSeeOther && rec.Header().Get("Location") != "/login" {
				t.Errorf("Location = %q, want /login", rec.Header().Get("Location"))
			}
		})
	}
}

func TestRequireUserRenewsCookie(t *testing.T) {
	t.Parallel()

	renewed := testNow.Add(90 * 24 * time.Hour)
	uc := mockUsecase(t)
	uc.EXPECT().Authenticate(gomock.Any(), testToken).
		Return(usecase.Session{Token: testToken, UserID: testUserID, ExpiresAt: renewed, Renewed: true}, nil)
	uc.EXPECT().ListCards(gomock.Any(), testUserID).Return(nil, nil)
	wb := newTestDelivery(t, uc)

	rec := do(t, wb, http.MethodGet, "/", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	// база продлила сессию - кука продлевается вместе с ней, иначе браузер выкинет её раньше
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != testToken || !cookies[0].Expires.Equal(renewed) {
		t.Errorf("cookies = %+v, want renewed session cookie", cookies)
	}
}

func TestLogin(t *testing.T) {
	t.Parallel()

	expires := testNow.Add(90 * 24 * time.Hour)
	tests := []struct {
		name       string
		form       url.Values
		loginErr   error
		wantStatus int
		wantError  string
	}{
		{name: "ok", form: url.Values{"login": {" me "}, "password": {"pw"}}, wantStatus: http.StatusSeeOther},
		{
			name: "wrong password", form: url.Values{"login": {"me"}, "password": {"nope"}},
			loginErr: usecase.ErrWrongPassword, wantStatus: http.StatusUnprocessableEntity,
			wantError: "Неверный логин или пароль",
		},
		{
			name: "empty", form: url.Values{"login": {""}, "password": {""}},
			wantStatus: http.StatusUnprocessableEntity, wantError: "Введите логин и пароль",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := mockUsecase(t)
			if tt.form.Get("password") != "" {
				// пробелы вокруг логина отрезаются: на телефоне их ставит автозамена
				uc.EXPECT().Login(gomock.Any(), "me", tt.form.Get("password")).
					Return(usecase.Session{Token: testToken, UserID: testUserID, ExpiresAt: expires}, tt.loginErr)
			}
			wb := newTestDelivery(t, uc)

			rec := do(t, wb, http.MethodPost, "/login", tt.form, false)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantError != "" {
				if !strings.Contains(rec.Body.String(), tt.wantError) {
					t.Errorf("body has no %q", tt.wantError)
				}
				return
			}

			cookies := rec.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("cookies = %+v", cookies)
			}
			c := cookies[0]
			if c.Name != sessionCookie || c.Value != testToken || !c.Expires.Equal(expires) ||
				!c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Domain != testDomain {
				t.Errorf("session cookie = %+v", c)
			}
		})
	}
}

func TestRegister(t *testing.T) {
	t.Parallel()

	expires := testNow.Add(90 * 24 * time.Hour)
	form := url.Values{"login": {" Sasha "}, "password": {"secret-pw"}, "password_confirm": {"secret-pw"}}
	in := usecase.RegisterInput{Login: " Sasha ", Password: "secret-pw", PasswordConfirm: "secret-pw"}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		uc := mockUsecase(t)
		uc.EXPECT().Register(gomock.Any(), in).
			Return(usecase.Session{Token: testToken, UserID: testUserID, ExpiresAt: expires}, nil)
		wb := newTestDelivery(t, uc)

		rec := do(t, wb, http.MethodPost, "/register", form, false)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
			t.Fatalf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
		}
		// после регистрации сразу вошли
		cookies := rec.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Value != testToken || !cookies[0].Expires.Equal(expires) {
			t.Errorf("cookies = %+v", cookies)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()

		uc := mockUsecase(t)
		invalid := usecase.ValidationError{usecase.FieldLogin: usecase.ErrLoginTaken}
		uc.EXPECT().Register(gomock.Any(), in).Return(usecase.Session{}, invalid)
		wb := newTestDelivery(t, uc)

		rec := do(t, wb, http.MethodPost, "/register", form, false)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Такой логин уже занят") || !strings.Contains(body, `value="Sasha"`) {
			t.Error("form must show the error and keep the login")
		}
		// пароль обратно в разметку не попадает
		if strings.Contains(body, "secret-pw") {
			t.Error("password leaked into the page")
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Error("failed registration must not log in")
		}
	})
}

func TestRegisterPage(t *testing.T) {
	t.Parallel()

	uc := mockUsecase(t)
	expectSession(uc)
	wb := newTestDelivery(t, uc)

	// на входе под кнопкой - строка со ссылкой на регистрацию, и обратно
	rec := do(t, wb, http.MethodGet, "/login", nil, false)
	toRegister := `<p class="auth-switch">Нет аккаунта? <a href="/register">Зарегистрироваться</a></p>`
	if !strings.Contains(rec.Body.String(), toRegister) {
		t.Error("login page has no link to registration")
	}

	rec = do(t, wb, http.MethodGet, "/register", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `action="/register"`) {
		t.Errorf("register page: status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `<p class="auth-switch">Уже есть аккаунт? <a href="/login">Войти</a></p>`) {
		t.Error("register page has no link to login")
	}

	// уже вошедшему регистрация не нужна
	rec = do(t, wb, http.MethodGet, "/register", nil, true)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("logged in: status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestLogout(t *testing.T) {
	t.Parallel()

	uc := mockUsecase(t)
	uc.EXPECT().Logout(gomock.Any(), testToken).Return(nil)
	wb := newTestDelivery(t, uc)

	rec := do(t, wb, http.MethodPost, "/logout", url.Values{}, true)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	// офлайн-копии карт стирает service worker. Clear-Site-Data не нужен: Chrome с ним подвешивает выход
	// и выкидывает всю статику
	if got := rec.Header().Get("Clear-Site-Data"); got != "" {
		t.Errorf("Clear-Site-Data = %q, want none", got)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != "" || cookies[0].Expires.After(time.Unix(0, 0)) {
		t.Errorf("cookies = %+v, want expired session cookie", cookies)
	}
}

func TestCardsPage(t *testing.T) {
	t.Parallel()

	uc := mockUsecase(t)
	expectSession(uc)
	withNote := testCard(1, "Пятёрочка")
	withNote.Note = "PIN 4417"
	uc.EXPECT().ListCards(gomock.Any(), testUserID).
		Return([]model.Cards{withNote, testCard(2, "<script>Лента</script>")}, nil)
	wb := newTestDelivery(t, uc)

	rec := do(t, wb, http.MethodGet, "/", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"Пятёрочка",
		// название экранируется: скрипт из названия карты не выполнится
		"&lt;script&gt;Лента&lt;/script&gt;",
		// во второй строке - заметка, а не содержимое кода
		`<span class="deck-card-note">PIN 4417</span>`,
		// масть рисунком из спрайта, а не символом
		`href="#suit-`,
		// индексы с обеих сторон карты
		`<span class="pip pip-end" aria-hidden="true"><span class="pip-letter">П</span>`,
		// адреса для офлайн-кэша: страница карты и картинка кода с версией
		`data-offline="/cards/1 /cards/1/qr.png?v=`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body has no %q", want)
		}
	}
	// содержимое кода в колоде не показываем, и у карты без заметки вторая строка пустая
	if strings.Contains(body, "2775") || strings.Count(body, `class="deck-card-note"`) != 1 {
		t.Error("deck must show notes, not qr content")
	}
	if strings.Contains(body, "<script>Лента") {
		t.Error("card name is not escaped")
	}
	// тема ставится до первой отрисовки: theme.js в <head> без defer и раньше стилей
	themeJS := strings.Index(body, `<script src="/static/theme.js?v=`)
	if themeJS < 0 || themeJS > strings.Index(body, `/static/app.css?v=`) {
		t.Error("theme.js must be loaded in head before app.css")
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
		t.Errorf("Content-Security-Policy = %q", got)
	}
	// выход - в колонтитуле, а не внизу под колодой
	if !strings.Contains(body, `<button class="bar-action" type="submit" data-online-only>Выйти</button>`) {
		t.Error("cards page has no logout button in the header")
	}
	// тумблер темы - там же, в колонтитуле, перед «Выйти»
	toggle := strings.Index(body, `role="switch" aria-checked="false" aria-label="Тёмная тема" data-theme-toggle`)
	if toggle < 0 || toggle > strings.Index(body, `action="/logout"`) {
		t.Error("cards page has no theme toggle before logout")
	}
	// добавление без сети тоже недоступно: формы нет в офлайн-кэше
	if !strings.Contains(body, `href="/cards/new" data-online-only>Добавить карту</a>`) {
		t.Error("add button is not marked as online only")
	}
	// список карт персональный: после выхода в кэше браузера его не остаётся
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestCardPage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefetch bool
	}{
		// открыли на кассе: карта поднимается наверх списка
		{name: "open"},
		// service worker заранее кладёт карту в кэш: это не открытие
		{name: "prefetch", prefetch: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := mockUsecase(t)
			expectSession(uc)
			card := testCard(3, "Пятёрочка")
			if tt.prefetch {
				uc.EXPECT().GetCard(gomock.Any(), testUserID, int64(3)).Return(card, nil)
			} else {
				uc.EXPECT().OpenCard(gomock.Any(), testUserID, int64(3)).Return(card, nil)
			}
			wb := newTestDelivery(t, uc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/cards/3", nil)
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: testToken})
			if tt.prefetch {
				req.Header.Set(prefetchHeader, "1")
			}
			rec := httptest.NewRecorder()
			wb.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "/cards/3/qr.png?v=") {
				t.Error("card page has no qr image")
			}
		})
	}
}

func TestCardNotFound(t *testing.T) {
	t.Parallel()

	uc := mockUsecase(t)
	expectSession(uc)
	uc.EXPECT().OpenCard(gomock.Any(), testUserID, int64(9)).Return(model.Cards{}, usecase.ErrNotFound)
	uc.EXPECT().CardQR(gomock.Any(), testUserID, int64(9)).Return(nil, usecase.ErrNotFound)
	wb := newTestDelivery(t, uc)

	for _, target := range []string{"/cards/9", "/cards/9/qr.png", "/cards/abc"} {
		rec := do(t, wb, http.MethodGet, target, nil, true)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, rec.Code)
		}
	}
}

func TestCardQR(t *testing.T) {
	t.Parallel()

	uc := mockUsecase(t)
	expectSession(uc)
	uc.EXPECT().CardQR(gomock.Any(), testUserID, int64(3)).Return([]byte("png"), nil)
	wb := newTestDelivery(t, uc)

	rec := do(t, wb, http.MethodGet, "/cards/3/qr.png?v=1", nil, true)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != "png" {
		t.Fatalf("status = %d, Content-Type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	// код карты персональный: в кэше браузера его быть не должно
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestCreateCard(t *testing.T) {
	t.Parallel()

	form := url.Values{"name": {"Пятёрочка"}, "content": {"123"}, "note": {"PIN"}}
	in := usecase.CardInput{Name: "Пятёрочка", Content: "123", Note: "PIN"}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		uc := mockUsecase(t)
		expectSession(uc)
		uc.EXPECT().CreateCard(gomock.Any(), testUserID, in).Return(int64(5), nil)
		wb := newTestDelivery(t, uc)

		rec := do(t, wb, http.MethodPost, "/cards", form, true)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/cards/5" {
			t.Errorf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()

		uc := mockUsecase(t)
		expectSession(uc)
		invalid := usecase.ValidationError{usecase.FieldName: usecase.ErrRequired}
		uc.EXPECT().CreateCard(gomock.Any(), testUserID, in).Return(int64(0), invalid)
		wb := newTestDelivery(t, uc)

		rec := do(t, wb, http.MethodPost, "/cards", form, true)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d", rec.Code)
		}
		// форма вернулась с подсказкой и введёнными значениями
		body := rec.Body.String()
		for _, want := range []string{"Как называется магазин?", ">123</textarea>", `value="PIN"`} {
			if !strings.Contains(body, want) {
				t.Errorf("body has no %q", want)
			}
		}
	})
}

func TestCreateCardExists(t *testing.T) {
	t.Parallel()

	form := url.Values{"name": {"Лента"}, "content": {"123"}}
	in := usecase.CardInput{Name: "Лента", Content: "123"}

	tests := []struct {
		name     string
		err      error
		wantLink bool
	}{
		{name: "known card", err: usecase.CardExistsError{ID: 3, Name: "Пятёрочка"}, wantLink: true},
		// карту удалили, пока искали: сказать, что дубль, можно, сослаться - не на что
		{name: "unknown card", err: usecase.ErrCardExists},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := mockUsecase(t)
			expectSession(uc)
			uc.EXPECT().CreateCard(gomock.Any(), testUserID, in).Return(int64(0), tt.err)
			wb := newTestDelivery(t, uc)

			rec := do(t, wb, http.MethodPost, "/cards", form, true)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "Такая карта уже есть в колоде") {
				t.Error("body has no duplicate message")
			}
			link := `<a href="/cards/3">«Пятёрочка»</a>`
			if strings.Contains(body, link) != tt.wantLink {
				t.Errorf("link to existing card shown = %v, want %v", !tt.wantLink, tt.wantLink)
			}
		})
	}
}

func TestCardButtons(t *testing.T) {
	t.Parallel()

	uc := mockUsecase(t)
	expectSession(uc)
	card := testCard(3, "Пятёрочка")
	uc.EXPECT().OpenCard(gomock.Any(), testUserID, int64(3)).Return(card, nil)
	uc.EXPECT().GetCard(gomock.Any(), testUserID, int64(3)).Return(card, nil).Times(2)
	wb := newTestDelivery(t, uc)

	// на странице карты: «Изменить» над «Удалить»
	body := do(t, wb, http.MethodGet, "/cards/3", nil, true).Body.String()
	// без сети обе кнопки недоступны (data-online-only): правки и удаления в офлайн-кэше нет
	edit := strings.Index(body, `<a class="btn btn-secondary" href="/cards/3/edit" data-online-only>Изменить карту</a>`)
	del := strings.Index(body,
		`<a class="btn btn-danger" href="/cards/3/delete" data-dialog="delete-dialog" data-online-only>Удалить карту</a>`)
	if edit < 0 || del < 0 || edit > del {
		t.Errorf("card page: edit button at %d, delete button at %d, want edit above delete", edit, del)
	}
	// подтверждение - окном на той же странице, удаление уходит на сервер формой
	for _, want := range []string{
		`<dialog class="modal" id="delete-dialog"`,
		`Удалить «Пятёрочка»?`,
		`<form method="post" action="/cards/3/delete">
        <button class="btn btn-danger" type="submit" data-online-only>Удалить</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("card page has no %s", want)
		}
	}

	// в правке удаления нет: удаляют только со страницы карты
	body = do(t, wb, http.MethodGet, "/cards/3/edit", nil, true).Body.String()
	if strings.Contains(body, "/cards/3/delete") {
		t.Error("edit page must not link to delete")
	}

	// с подтверждения - обратно на карту
	body = do(t, wb, http.MethodGet, "/cards/3/delete", nil, true).Body.String()
	if !strings.Contains(body, `<a class="btn btn-secondary" href="/cards/3">Оставить</a>`) {
		t.Error("delete page has no way back to the card")
	}
}

func TestUpdateAndDeleteCard(t *testing.T) {
	t.Parallel()

	uc := mockUsecase(t)
	expectSession(uc)
	in := usecase.CardInput{Name: "Лента", Content: "1"}
	uc.EXPECT().UpdateCard(gomock.Any(), testUserID, int64(3), in).Return(nil)
	uc.EXPECT().UpdateCard(gomock.Any(), testUserID, int64(4), in).Return(usecase.ErrNotFound)
	uc.EXPECT().DeleteCard(gomock.Any(), testUserID, int64(3)).Return(nil)
	uc.EXPECT().DeleteCard(gomock.Any(), testUserID, int64(4)).Return(usecase.ErrNotFound)
	wb := newTestDelivery(t, uc)

	form := url.Values{"name": {"Лента"}, "content": {"1"}}
	rec := do(t, wb, http.MethodPost, "/cards/3", form, true)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/cards/3" {
		t.Errorf("update: status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = do(t, wb, http.MethodPost, "/cards/4", form, true)
	if rec.Code != http.StatusNotFound {
		t.Errorf("update of foreign card: status = %d", rec.Code)
	}

	rec = do(t, wb, http.MethodPost, "/cards/3/delete", url.Values{}, true)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("delete: status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = do(t, wb, http.MethodPost, "/cards/4/delete", url.Values{}, true)
	if rec.Code != http.StatusNotFound {
		t.Errorf("delete of foreign card: status = %d", rec.Code)
	}
}

// upload - multipart-запрос распознавания с файлом data
func upload(t *testing.T, wb http.Handler, data []byte, htmx bool) *httptest.ResponseRecorder {
	t.Helper()

	var body strings.Builder
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("image", "card.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(data)
	_ = mw.Close()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/cards/scan", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: testToken})
	if htmx {
		req.Header.Set("Hx-Request", "true")
	}

	rec := httptest.NewRecorder()
	wb.ServeHTTP(rec, req)

	return rec
}

func TestScanCard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		scanned    string
		scanErr    error
		htmx       bool
		wantStatus int
		want       string
	}{
		{name: "recognized", scanned: "2775012345678", wantStatus: http.StatusOK, want: ">2775012345678</textarea>"},
		{name: "no qr", scanErr: usecase.ErrQRNotFound, wantStatus: http.StatusOK, want: "QR не распознан"},
		{
			name: "not an image", scanErr: usecase.ErrBadImage,
			wantStatus: http.StatusUnprocessableEntity, want: "JPEG или PNG",
		},
		// htmx получает только содержимое страницы, без каркаса
		{name: "htmx", scanned: "1", htmx: true, wantStatus: http.StatusOK, want: ">1</textarea>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := mockUsecase(t)
			expectSession(uc)
			uc.EXPECT().ScanQR(gomock.Any(), gomock.Any()).Return(tt.scanned, tt.scanErr)
			wb := newTestDelivery(t, uc)

			rec := upload(t, wb, []byte("image"), tt.htmx)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.want) {
				t.Errorf("body has no %q", tt.want)
			}
			if hasLayout := strings.Contains(body, "<!doctype html>"); hasLayout == tt.htmx {
				t.Errorf("layout rendered = %v for htmx = %v", hasLayout, tt.htmx)
			}
		})
	}
}

func TestScanCardTooBig(t *testing.T) {
	t.Parallel()

	// до распознавания дело не доходит: ScanQR не ожидается
	uc := mockUsecase(t)
	expectSession(uc)
	wb := newTestDelivery(t, uc)

	rec := upload(t, wb, make([]byte, maxUpload+1), false)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "больше 10 МБ") {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestPWAFiles(t *testing.T) {
	t.Parallel()

	wb := newTestDelivery(t, mockUsecase(t))

	for target, contentType := range map[string]string{
		"/sw.js":                "text/javascript; charset=utf-8",
		"/manifest.webmanifest": "application/manifest+json",
	} {
		rec := do(t, wb, http.MethodGet, target, nil, false)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != contentType {
			t.Errorf("%s: status = %d, Content-Type = %q", target, rec.Code, rec.Header().Get("Content-Type"))
		}
		// без версии в адресе: браузер обязан сверять файл при каждой загрузке
		if rec.Header().Get("Cache-Control") != cacheRevalidate {
			t.Errorf("%s: Cache-Control = %q", target, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestCardCode(t *testing.T) {
	t.Parallel()

	for content, want := range map[string]string{
		"2775012345678":       "2775 0123 4567 8",
		"1234":                "1234",
		"12345":               "1234 5",
		"https://example.com": "https://example.com",
		"AB12345678":          "AB12345678",
	} {
		if got := cardCode(content); got != want {
			t.Errorf("cardCode(%q) = %q, want %q", content, got, want)
		}
	}
}

func TestCardIndex(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{
		"пятёрочка": "П",
		"«Лента»":   "Л",
		"7я":        "7",
		"...":       "?",
	} {
		if got := cardIndex(name); got != want {
			t.Errorf("cardIndex(%q) = %q, want %q", name, got, want)
		}
	}
}
