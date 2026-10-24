// uuidv7 - печатает новый UUID версии 7: id для пользователя, которого заводят руками,
// см. migrations/20261024100000_users.sql
package main

import (
	"fmt"
	"os"

	"github.com/google/uuid"
)

func main() {
	id, err := uuid.NewV7()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "can't generate uuid:", err)
		os.Exit(1)
	}

	// утилита для терминала: её вывод и есть результат
	_, _ = fmt.Fprintln(os.Stdout, id)
}
