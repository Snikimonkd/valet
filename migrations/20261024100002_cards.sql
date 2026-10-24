-- +goose Up
-- +goose StatementBegin
CREATE TABLE cards (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id uuid NOT NULL,
    -- название магазина
    name text NOT NULL,
    -- что зашито в код: номер карты, ссылка, что угодно
    content text NOT NULL,
    -- формат кода, допустимые значения перечислены в usecase.CardFormat
    format text NOT NULL,
    -- заметка: PIN, телефон, к которому привязана карта
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    -- когда карту последний раз открывали на кассе. У новой карты - время создания
    last_used_at timestamptz NOT NULL
);

-- список карт пользователя: недавно открытые сверху, индекс читается в обратном порядке
CREATE INDEX cards_user_id_last_used_at_idx ON cards(user_id, last_used_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE cards;
-- +goose StatementEnd
