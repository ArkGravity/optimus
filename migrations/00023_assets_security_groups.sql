-- +goose Up
CREATE TABLE aws_security_groups (
    id BIGSERIAL PRIMARY KEY,
    cloud_account_id BIGINT NOT NULL REFERENCES cloud_accounts(id),
    region VARCHAR(32) NOT NULL,
    group_id VARCHAR(64) NOT NULL,
    group_name TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    vpc_id VARCHAR(64) NOT NULL DEFAULT '',
    owner_id VARCHAR(32) NOT NULL DEFAULT '',
    inbound_rules JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(inbound_rules) = 'array'),
    outbound_rules JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(outbound_rules) = 'array'),
    tags JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(tags) = 'object'),
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aws_security_groups_keytuple ON aws_security_groups(cloud_account_id, region, group_id);
CREATE INDEX idx_aws_security_groups_vpc ON aws_security_groups(cloud_account_id, region, vpc_id);
CREATE INDEX idx_aws_security_groups_last_seen ON aws_security_groups(last_seen_at);

-- +goose Down
DROP TABLE aws_security_groups;
