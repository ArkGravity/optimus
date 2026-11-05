-- +goose Up
CREATE TABLE menus (
    id              BIGSERIAL PRIMARY KEY,
    parent_id       BIGINT,
    code            VARCHAR(64)  NOT NULL,
    name            VARCHAR(128) NOT NULL,
    path            VARCHAR(255) NOT NULL DEFAULT '',
    component       VARCHAR(255) NOT NULL DEFAULT '',
    icon            VARCHAR(64)  NOT NULL DEFAULT '',
    permission_code VARCHAR(128),
    sort_order      INT          NOT NULL DEFAULT 0,
    hidden          BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT fk_menus_parent FOREIGN KEY (parent_id) REFERENCES menus(id) ON DELETE RESTRICT
);

CREATE INDEX idx_menus_parent_id  ON menus (parent_id);
CREATE INDEX idx_menus_deleted_at ON menus (deleted_at);

CREATE UNIQUE INDEX menus_code_uniq ON menus (code) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE menus;
