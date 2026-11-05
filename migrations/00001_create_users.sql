-- +goose Up
CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    username        VARCHAR(64)  NOT NULL,
    email           VARCHAR(128) NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    display_name    VARCHAR(128) NOT NULL DEFAULT '',
    avatar_url      VARCHAR(512) NOT NULL DEFAULT '',
    status          VARCHAR(16)  NOT NULL DEFAULT 'enabled',
    last_login_at   TIMESTAMPTZ,
    created_by      BIGINT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_users_deleted_at ON users (deleted_at);

CREATE UNIQUE INDEX users_username_uniq ON users (username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_email_uniq ON users (email) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE users;
