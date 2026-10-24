package repository

import (
	"testing"
	"time"

	"valet/internal/pkg/testsupport"
)

// время в тестах округлено до микросекунд: точнее postgres не хранит
func testTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
}

func newTestRepository(t *testing.T) repository {
	t.Helper()

	return NewRepository(testsupport.ConnectToTestPostgres(t.Context(), t))
}
