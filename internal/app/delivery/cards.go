package delivery

import (
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
)

// maxUpload - самое большое фото или скриншот, который принимаем. Фото с камеры телефона - 3-8 МБ
const maxUpload = 10 << 20

// prefetchHeader - так service worker помечает запросы, которыми заранее кладёт карты в офлайн-кэш.
// Такой запрос не считается открытием карты на кассе и не поднимает её наверх списка
const prefetchHeader = "X-Valet-Prefetch"

// suits - масти для уголка карты в списке: просто чтобы соседние карты различались на глаз.
// Значение - имя рисунка в спрайте (partials.html): символы ♠♥ телефон может подменить эмодзи
var suits = [...]string{"spade", "heart", "diamond", "club"}

// locMsk - московское время для подписей «09:12», «вчера». Летнего времени в Москве нет,
// поэтому хватает постоянного сдвига и не нужна база часовых поясов в образе
var locMsk = time.FixedZone("MSK", 3*60*60)

// cardView - карта для шаблонов
type cardView struct {
	ID      int64
	Name    string
	Content string
	Note    string
	// Index - буква в уголке карты: первая буква названия
	Index string
	// Suit - масть в уголке, выбирается по названию и не меняется
	Suit string
	// Code - содержимое для подписи: номер из цифр группами по четыре, остальное как есть
	Code string
	// Used - когда карту показывали: «09:12» сегодня, «вчера», «21.10». У ни разу не открытой - «новая»
	Used string
	// UsedAt - то же время для атрибута datetime
	UsedAt time.Time
	// QRURL - адрес картинки с кодом. Версия в адресе меняется при правке карты, поэтому картинку можно кэшировать
	QRURL string
}

func newCardView(c model.Cards, now time.Time) cardView {
	h := fnv.New32a()
	_, _ = h.Write([]byte(c.Name))

	return cardView{
		ID:      c.ID,
		Name:    c.Name,
		Content: c.Content,
		Note:    c.Note,
		Index:   cardIndex(c.Name),
		Suit:    suits[h.Sum32()%uint32(len(suits))],
		Code:    cardCode(c.Content),
		Used:    usedLabel(c, now),
		UsedAt:  c.LastUsedAt,
		QRURL:   fmt.Sprintf("/cards/%d/qr.png?v=%d", c.ID, c.UpdatedAt.UnixMilli()),
	}
}

// URL - страница карты
func (c cardView) URL() string {
	return "/cards/" + strconv.FormatInt(c.ID, 10)
}

// OfflineURLs - что service worker заранее кладёт в кэш для карты: её страницу и картинку с кодом
func (c cardView) OfflineURLs() string {
	return strings.Join([]string{c.URL(), c.QRURL}, " ")
}

// cardIndex - первая буква или цифра названия заглавной: «Пятёрочка» → «П», «7я» → «7»
func cardIndex(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}

	return "?"
}

// cardCode - номер карты из одних цифр группами по четыре, как на пластике: «2775 0841 3390 12».
// Его вбивают руками, если сканер не прочитал код. Остальное содержимое - как есть
func cardCode(content string) string {
	const group = 4
	for _, r := range content {
		if r < '0' || r > '9' {
			return content
		}
	}

	var b strings.Builder
	for i, r := range content {
		if i > 0 && i%group == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}

	return b.String()
}

// usedLabel - когда карту последний раз показывали, коротко для списка. Только что созданную
// ещё не показывали: у неё время показа равно времени создания
func usedLabel(c model.Cards, now time.Time) string {
	if c.LastUsedAt.Equal(c.CreatedAt) {
		return "новая"
	}

	used, now := c.LastUsedAt.In(locMsk), now.In(locMsk)
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, locMsk) }
	days := int(day(now).Sub(day(used)) / (24 * time.Hour))
	switch days {
	case 0:
		return used.Format("15:04")
	case 1:
		return "вчера"
	default:
		return used.Format("02.01")
	}
}

// usedText - то же для страницы правки: «сегодня в 09:12», «вчера в 18:40», «21.10 в 12:00»
func usedText(c model.Cards, now time.Time) string {
	label := usedLabel(c, now)
	if label == "новая" {
		return "ещё не показывали"
	}

	at := c.LastUsedAt.In(locMsk).Format("15:04")
	switch label {
	case at:
		return "сегодня в " + at
	case "вчера":
		return "вчера в " + at
	default:
		return label + " в " + at
	}
}

// cardsCount - «1 карта», «3 карты», «7 карт»
func cardsCount(n int) string {
	word := "карт"
	switch {
	case n%100 >= 11 && n%100 <= 14:
	case n%10 == 1:
		word = "карта"
	case n%10 >= 2 && n%10 <= 4:
		word = "карты"
	}

	return strconv.Itoa(n) + " " + word
}

type cardsView struct {
	Cards []cardView
	// Count - сколько карт в колоде, для колонцифры
	Count string
}

func (wb *delivery) cardsPage(w http.ResponseWriter, r *http.Request) {
	cards, err := wb.usecase.ListCards(r.Context(), userID(r.Context()))
	if err != nil {
		wb.serverError(w, r, err)
		return
	}

	now := time.Now()
	view := cardsView{Cards: make([]cardView, 0, len(cards)), Count: cardsCount(len(cards))}
	for _, c := range cards {
		view.Cards = append(view.Cards, newCardView(c, now))
	}

	wb.renderPage(w, r, http.StatusOK, pageCards, view)
}

// cardID - id карты из адреса. Ложь, если в адресе не число
func cardID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	return id, err == nil && id > 0
}

// cardNotFound - карты нет или она чужая: ответ одинаковый, о чужих картах ничего не узнать
func cardNotFound(w http.ResponseWriter) {
	http.Error(w, "Такой карты нет", http.StatusNotFound)
}

// loadCard - карта пользователя из адреса запроса для страниц, которые её не открывают на кассе.
// Ложь, если ответ уже отправлен: карты нет или база не ответила
func (wb *delivery) loadCard(w http.ResponseWriter, r *http.Request) (model.Cards, bool) {
	id, ok := cardID(r)
	if !ok {
		cardNotFound(w)
		return model.Cards{}, false
	}

	card, err := wb.usecase.GetCard(r.Context(), userID(r.Context()), id)
	if errors.Is(err, usecase.ErrNotFound) {
		cardNotFound(w)
		return model.Cards{}, false
	}
	if err != nil {
		wb.serverError(w, r, err)
		return model.Cards{}, false
	}

	return card, true
}

func (wb *delivery) cardPage(w http.ResponseWriter, r *http.Request) {
	id, ok := cardID(r)
	if !ok {
		cardNotFound(w)
		return
	}

	open := wb.usecase.OpenCard
	if r.Header.Get(prefetchHeader) != "" {
		open = wb.usecase.GetCard
	}

	card, err := open(r.Context(), userID(r.Context()), id)
	if errors.Is(err, usecase.ErrNotFound) {
		cardNotFound(w)
		return
	}
	if err != nil {
		wb.serverError(w, r, err)
		return
	}

	wb.renderPage(w, r, http.StatusOK, pageCard, newCardView(card, time.Now()))
}

func (wb *delivery) cardQR(w http.ResponseWriter, r *http.Request) {
	id, ok := cardID(r)
	if !ok {
		cardNotFound(w)
		return
	}

	png, err := wb.usecase.CardQR(r.Context(), userID(r.Context()), id)
	if errors.Is(err, usecase.ErrNotFound) {
		cardNotFound(w)
		return
	}
	if err != nil {
		wb.serverError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	// код карты персональный, как и страницы: в кэше браузера не храним. Офлайн-копию держит service worker,
	// а версия в адресе (после правки карты он новый) позволяет ему не перекачивать картинку
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png) //nolint:gosec // это PNG от qr.Encode, браузер не примет его за html: Content-Type и nosniff
}

// cardFormView - форма карты: новой (ID 0) или существующей
type cardFormView struct {
	ID      int64
	Name    string
	Content string
	Note    string
	// Scanned - содержимое прочитано с картинки, а не введено руками
	Scanned bool
	// ScanFailed - на картинке не нашлось кода, содержимое вводят руками
	ScanFailed bool
	// Used - когда карту последний раз показывали, только у существующей
	Used   string
	Errors map[string]string
	// Existing - карта с таким же содержимым, которая уже есть в колоде: на неё ведёт ссылка в ошибке
	Existing *cardRef
}

// cardRef - ссылка на карту: id и название
type cardRef struct {
	ID   int64
	Name string
}

// Action - куда отправляется форма
func (v cardFormView) Action() string {
	if v.ID == 0 {
		return "/cards"
	}

	return "/cards/" + strconv.FormatInt(v.ID, 10)
}

// duplicateMsg - подсказка у содержимого, если такая карта уже есть
const duplicateMsg = "Такая карта уже есть в колоде"

// withSaveError - форма с ошибкой сохранения err. Ложь, если err - не ошибка ввода и отвечать надо иначе
func (v cardFormView) withSaveError(err error) (cardFormView, bool) {
	if invalid, ok := errors.AsType[usecase.ValidationError](err); ok {
		v.Errors = cardMessages.messages(invalid)
		return v, true
	}
	if !errors.Is(err, usecase.ErrCardExists) {
		return v, false
	}

	v.Errors = map[string]string{usecase.FieldContent: duplicateMsg}
	// какую карту повторили, известно не всегда: её могли удалить, пока искали
	if exists, ok := errors.AsType[usecase.CardExistsError](err); ok {
		v.Existing = &cardRef{ID: exists.ID, Name: exists.Name}
	}

	return v, true
}

// cardMessages - подсказки у полей формы карты на нарушенные правила usecase
var cardMessages = ruleMessages{
	usecase.FieldName: {
		usecase.ErrRequired: "Как называется магазин?",
		usecase.ErrTooLong:  "Слишком длинное название",
	},
	usecase.FieldContent: {
		usecase.ErrRequired: "Что зашито в код: номер карты или текст под QR-кодом",
		usecase.ErrTooLong:  "Столько не влезет в один QR-код",
	},
	usecase.FieldNote: {
		usecase.ErrTooLong: "Слишком длинная заметка",
	},
}

func cardInput(r *http.Request) usecase.CardInput {
	return usecase.CardInput{
		Name:    r.PostFormValue("name"),
		Content: r.PostFormValue("content"),
		Note:    r.PostFormValue("note"),
	}
}

type cardNewView struct {
	Error string
}

func (wb *delivery) newCardPage(w http.ResponseWriter, r *http.Request) {
	wb.renderPage(w, r, http.StatusOK, pageCardNew, cardNewView{})
}

func (wb *delivery) manualCardPage(w http.ResponseWriter, r *http.Request) {
	wb.renderPage(w, r, http.StatusOK, pageCardForm, cardFormView{})
}

// scanCard - фото или скриншот карты: читает QR-код и показывает форму с прочитанным содержимым.
// Кода не нашлось - та же форма с пустым содержимым для ручного ввода
func (wb *delivery) scanCard(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)

	file, _, err := r.FormFile("image")
	if err != nil {
		msg := "Не получилось загрузить картинку, попробуйте ещё раз"
		if _, tooBig := errors.AsType[*http.MaxBytesError](err); tooBig {
			msg = "Картинка больше 10 МБ, нужна поменьше"
		}
		wb.renderContent(w, r, http.StatusUnprocessableEntity, pageCardNew, cardNewView{Error: msg})
		return
	}
	defer file.Close()

	content, err := wb.usecase.ScanQR(r.Context(), file)
	switch {
	case errors.Is(err, usecase.ErrBadImage):
		view := cardNewView{Error: "Это не похоже на фото или скриншот: нужна картинка JPEG или PNG"}
		wb.renderContent(w, r, http.StatusUnprocessableEntity, pageCardNew, view)
	case errors.Is(err, usecase.ErrQRNotFound):
		wb.renderContent(w, r, http.StatusOK, pageCardForm, cardFormView{ScanFailed: true})
	case err != nil:
		wb.serverError(w, r, err)
	default:
		wb.renderContent(w, r, http.StatusOK, pageCardForm, cardFormView{Content: content, Scanned: true})
	}
}

func (wb *delivery) createCard(w http.ResponseWriter, r *http.Request) {
	in := cardInput(r)

	id, err := wb.usecase.CreateCard(r.Context(), userID(r.Context()), in)
	view, invalid := cardFormView{Name: in.Name, Content: in.Content, Note: in.Note}.withSaveError(err)
	if invalid {
		wb.renderPage(w, r, http.StatusUnprocessableEntity, pageCardForm, view)
		return
	}
	if err != nil {
		wb.serverError(w, r, err)
		return
	}

	redirect(w, r, "/cards/"+strconv.FormatInt(id, 10))
}

func (wb *delivery) editCardPage(w http.ResponseWriter, r *http.Request) {
	card, ok := wb.loadCard(w, r)
	if !ok {
		return
	}

	wb.renderPage(w, r, http.StatusOK, pageCardForm, cardFormView{
		ID:      card.ID,
		Name:    card.Name,
		Content: card.Content,
		Note:    card.Note,
		Used:    usedText(card, time.Now()),
	})
}

func (wb *delivery) updateCard(w http.ResponseWriter, r *http.Request) {
	id, ok := cardID(r)
	if !ok {
		cardNotFound(w)
		return
	}
	in := cardInput(r)

	err := wb.usecase.UpdateCard(r.Context(), userID(r.Context()), id, in)
	view, invalid := cardFormView{ID: id, Name: in.Name, Content: in.Content, Note: in.Note}.withSaveError(err)
	if invalid {
		wb.renderPage(w, r, http.StatusUnprocessableEntity, pageCardForm, view)
		return
	}
	if errors.Is(err, usecase.ErrNotFound) {
		cardNotFound(w)
		return
	}
	if err != nil {
		wb.serverError(w, r, err)
		return
	}

	redirect(w, r, "/cards/"+strconv.FormatInt(id, 10))
}

// deleteCardPage - подтверждение удаления. Сюда ведут со страницы карты, «Назад» и «Оставить» возвращают на неё
func (wb *delivery) deleteCardPage(w http.ResponseWriter, r *http.Request) {
	card, ok := wb.loadCard(w, r)
	if !ok {
		return
	}

	wb.renderPage(w, r, http.StatusOK, pageCardDelete, newCardView(card, time.Now()))
}

func (wb *delivery) deleteCard(w http.ResponseWriter, r *http.Request) {
	id, ok := cardID(r)
	if !ok {
		cardNotFound(w)
		return
	}

	err := wb.usecase.DeleteCard(r.Context(), userID(r.Context()), id)
	if errors.Is(err, usecase.ErrNotFound) {
		cardNotFound(w)
		return
	}
	if err != nil {
		wb.serverError(w, r, err)
		return
	}

	redirect(w, r, "/")
}

// ruleMessages - подсказки у полей формы на нарушенные правила usecase: поле → правило → подсказка
type ruleMessages map[string]map[error]string

// messages - подсказки у полей для нарушений invalid. На правило без своей подсказки - общая
func (m ruleMessages) messages(invalid usecase.ValidationError) map[string]string {
	res := make(map[string]string, len(invalid))
	for field, rule := range invalid {
		msg, ok := m[field][rule]
		if !ok {
			msg = "Проверьте это поле"
		}
		res[field] = msg
	}

	return res
}
