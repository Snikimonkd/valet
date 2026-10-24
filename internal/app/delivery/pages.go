package delivery

// page - страница сайта; значение совпадает с именем шаблона в templates/pages без .html
type page string

const (
	pageLogin      page = "login"
	pageCards      page = "cards"
	pageCard       page = "card"
	pageCardNew    page = "card_new"
	pageCardForm   page = "card_form"
	pageCardDelete page = "card_delete"
)

// allPages - все страницы. parsePages проверяет, что у каждой есть шаблон и что лишних шаблонов нет
var allPages = []page{pageLogin, pageCards, pageCard, pageCardNew, pageCardForm, pageCardDelete}

func (p page) templatePath() string {
	return "templates/pages/" + string(p) + ".html"
}

// pageTitle - заголовок страницы во вкладке браузера
type pageTitle string

const (
	titleLogin      pageTitle = "Вход"
	titleCards      pageTitle = "Карты"
	titleCard       pageTitle = "Карта"
	titleCardNew    pageTitle = "Новая карта"
	titleCardForm   pageTitle = "Карта"
	titleCardDelete pageTitle = "Удалить карту"
)

// title - у каждой страницы заголовок один, поэтому он не передаётся при рендере отдельно
func (p page) title() pageTitle {
	switch p {
	case pageLogin:
		return titleLogin
	case pageCards:
		return titleCards
	case pageCard:
		return titleCard
	case pageCardNew:
		return titleCardNew
	case pageCardForm:
		return titleCardForm
	case pageCardDelete:
		return titleCardDelete
	}

	return ""
}
