-- +goose Up
CREATE TABLE apps_chart_repos (
    id                  BIGSERIAL    PRIMARY KEY,
    name                VARCHAR(64)  NOT NULL,
    type                VARCHAR(8)   NOT NULL CHECK (type IN ('oci','http')),
    url                 TEXT         NOT NULL,
    username            VARCHAR(255) NOT NULL DEFAULT '',
    encrypted_password  BYTEA        NOT NULL DEFAULT ''::BYTEA,
    description         TEXT         NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX apps_chart_repos_name_unique
    ON apps_chart_repos(name) WHERE deleted_at IS NULL;
CREATE INDEX apps_chart_repos_deleted_at ON apps_chart_repos(deleted_at);

-- +goose Down
DROP TABLE IF EXISTS apps_chart_repos;
