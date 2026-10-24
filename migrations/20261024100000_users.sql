-- +goose Up
-- +goose StatementBegin
-- пользователи. id - UUID версии 7, его генерирует приложение при регистрации.
-- Внешних ключей во всей схеме нет: связи между таблицами держит код
CREATE TABLE users (
    id uuid PRIMARY KEY,
    -- в нижнем регистре: вход без учёта регистра
    login text NOT NULL,
    -- bcrypt-хеш пароля
    password text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_login_unique_idx ON users(login);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE users;
-- +goose StatementEnd
