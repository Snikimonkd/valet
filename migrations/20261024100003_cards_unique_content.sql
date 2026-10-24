-- +goose Up
-- +goose StatementBegin
-- одну и ту же карту дважды не добавить: содержимое кода у карт пользователя не повторяется.
-- Если дубли уже есть, миграция упадёт - их надо убрать руками, молча удалять карты здесь нельзя
CREATE UNIQUE INDEX cards_user_id_content_unique_idx ON cards(user_id, content);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX cards_user_id_content_unique_idx;
-- +goose StatementEnd
