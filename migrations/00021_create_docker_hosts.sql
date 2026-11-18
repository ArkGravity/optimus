-- +goose Up
CREATE TABLE docker_hosts (
    id                 BIGSERIAL    PRIMARY KEY,
    name               VARCHAR(128) NOT NULL,
    description        TEXT         NOT NULL DEFAULT '',
    host               VARCHAR(255) NOT NULL,
    port               INTEGER      NOT NULL DEFAULT 22,
    socket_path        VARCHAR(255) NOT NULL DEFAULT '/var/run/docker.sock',
    auth_type          VARCHAR(16)  NOT NULL,
    ssh_key_id         BIGINT       REFERENCES credentials_ssh_keys(id) ON DELETE RESTRICT,
    username           VARCHAR(64)  NOT NULL,
    password_enc       BYTEA,
    last_checked_at    TIMESTAMPTZ,
    last_check_ok      BOOLEAN,
    last_check_error   TEXT         NOT NULL DEFAULT '',
    created_by_user_id BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at         TIMESTAMPTZ,
    CONSTRAINT docker_hosts_auth_shape CHECK (
        (auth_type = 'ssh_key' AND ssh_key_id IS NOT NULL AND password_enc IS NULL) OR
        (auth_type = 'password' AND ssh_key_id IS NULL AND password_enc IS NOT NULL)
    ),
    CONSTRAINT docker_hosts_port_range CHECK (port > 0 AND port <= 65535)
);

CREATE UNIQUE INDEX docker_hosts_name_unique
    ON docker_hosts(name) WHERE deleted_at IS NULL;
CREATE INDEX docker_hosts_ssh_key_id_idx
    ON docker_hosts(ssh_key_id) WHERE deleted_at IS NULL AND ssh_key_id IS NOT NULL;

-- +goose Down
DROP TABLE docker_hosts;
