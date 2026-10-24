package usecase

import (
	"errors"

	"github.com/benbjohnson/clock"
)

// Repository - все запросы к базе, которые нужны usecase
//
//go:generate go tool mockgen -typed -destination=mock/repository_mock.go -package=mock . Repository
type Repository interface {
	LoginRepository
	RegisterRepository
	SessionRepository
	CardsRepository
}

type usecase struct {
	repository Repository
	clock      clock.Clock
}

// NewUsecase - конструктор для usecase. clk - часы: в main настоящие, в тестах стоят на месте
func NewUsecase(repository Repository, clk clock.Clock) usecase {
	return usecase{
		repository: repository,
		clock:      clk,
	}
}

// ErrNotFound - нет такого пользователя, сессии или карты. Репозиторий отдаёт её вместо ошибок базы.
// Чужая карта для usecase тоже не найдена: о её существовании пользователь не узнает
var ErrNotFound = errors.New("not found error")
