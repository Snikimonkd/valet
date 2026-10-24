package usecase_test

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"valet/internal/app/usecase/mock"
)

// testNow - время на часах usecase в тестах
var testNow = time.Date(2026, time.October, 24, 9, 0, 0, 0, time.UTC)

// testUserID - пользователь, от имени которого работают тесты
var testUserID = uuid.MustParse("019a1d6e-0000-7000-8000-000000000007")

// mockClock - часы, которые стоят на at
func mockClock(at time.Time) *clock.Mock {
	c := clock.NewMock()
	c.Set(at)

	return c
}

// mockRepository - мок репозитория без ожиданий: вызов, которого тест не задал, роняет тест
func mockRepository(t *testing.T) *mock.MockRepository {
	t.Helper()

	return mock.NewMockRepository(gomock.NewController(t))
}
