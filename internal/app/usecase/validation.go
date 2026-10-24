package usecase

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

// Правила, которые может нарушить поле. Какое именно поле, говорит ValidationError
var (
	// ErrRequired - поле не заполнено
	ErrRequired = errors.New("required")
	// ErrTooLong - в поле больше символов, чем можно
	ErrTooLong = errors.New("too long")
)

// Поля входных структур, у которых бывают нарушения: ключи ValidationError
const (
	FieldName            = "Name"
	FieldContent         = "Content"
	FieldNote            = "Note"
	FieldLogin           = "Login"
	FieldPassword        = "Password"
	FieldPasswordConfirm = "PasswordConfirm"
)

// ValidationError - нарушения правил по полям: имя поля входной структуры → нарушенное правило.
// errors.Is находит в ней каждое нарушенное правило
type ValidationError map[string]error

func (e ValidationError) Error() string {
	fields := slices.Sorted(maps.Keys(e))
	for i, field := range fields {
		fields[i] = field + ": " + e[field].Error()
	}

	return "invalid " + strings.Join(fields, ", ")
}

func (e ValidationError) Unwrap() []error {
	return slices.Collect(maps.Values(e))
}

// check - отмечает у поля field нарушение rule, если ok ложно. У поля остаётся первое нарушение
func (e ValidationError) check(field string, ok bool, rule error) {
	if !ok && e[field] == nil {
		e[field] = rule
	}
}

// err - сама ошибка или nil, если нарушений нет
func (e ValidationError) err() error {
	if len(e) == 0 {
		return nil
	}

	return e
}

// filled - в строке есть что-то кроме пробелов
func filled(s string) bool {
	return strings.TrimSpace(s) != ""
}

// fits - в строке не больше limit символов
func fits(s string, limit int) bool {
	return utf8.RuneCountInString(s) <= limit
}
