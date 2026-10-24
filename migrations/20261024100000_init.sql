-- +goose Up
-- +goose StatementBegin
-- Внешних ключей нет: связи между таблицами держит код, каждый запрос к карте ограничен user_id.

-- пользователи. id - UUID версии 7, его генерирует приложение при регистрации
CREATE TABLE users (
    id uuid PRIMARY KEY,
    -- в нижнем регистре: вход без учёта регистра
    login text NOT NULL,
    -- bcrypt-хеш пароля
    password text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_login_unique_idx ON users(login);

-- сессии: токен из куки входа
CREATE TABLE sessions (
    token text PRIMARY KEY,
    user_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);

-- скидочные карты
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
-- одну и ту же карту дважды не добавить: содержимое кода у карт пользователя не повторяется
CREATE UNIQUE INDEX cards_user_id_content_unique_idx ON cards(user_id, content);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE cards;
DROP TABLE sessions;
DROP TABLE users;
-- +goose StatementEnd
