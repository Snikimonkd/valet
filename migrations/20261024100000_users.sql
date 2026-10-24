-- +goose Up
-- +goose StatementBegin
-- пользователей заводят руками, регистрации нет. id - UUID версии 7, его печатает go run ./cmd/uuidv7:
-- INSERT INTO users (id, login, password) VALUES ('<uuid>', 'me', '<bcrypt-хеш>');
CREATE TABLE users (
    id uuid PRIMARY KEY,
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
