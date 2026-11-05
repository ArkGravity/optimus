-- +goose Up
CREATE TABLE aliyun_instances (
    id BIGSERIAL PRIMARY KEY,
    cloud_account_id BIGINT NOT NULL REFERENCES cloud_accounts(id),
    instance_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    instance_type TEXT NOT NULL DEFAULT '',
    native_status TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL,
    zone TEXT NOT NULL DEFAULT '',
    network_interfaces JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(network_interfaces) = 'array'),
    tags JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(tags) = 'object'),
    native_details JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(native_details) = 'object'),
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aliyun_instances_keytuple ON aliyun_instances(cloud_account_id, region, instance_id);
CREATE INDEX idx_aliyun_instances_live ON aliyun_instances(cloud_account_id, region, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_aliyun_instances_last_seen ON aliyun_instances(last_seen_at);
CREATE INDEX idx_aliyun_instances_location ON aliyun_instances(cloud_account_id, region, zone);
CREATE INDEX idx_aliyun_instances_tags ON aliyun_instances USING GIN (tags);
CREATE INDEX idx_aliyun_instances_network_interfaces ON aliyun_instances USING GIN (network_interfaces);

CREATE TABLE aliyun_vpcs (
    id BIGSERIAL PRIMARY KEY,
    cloud_account_id BIGINT NOT NULL REFERENCES cloud_accounts(id),
    network_id TEXT NOT NULL,
    native_ref TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL,
    cidr_blocks JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(cidr_blocks) = 'array'),
    native_status TEXT NOT NULL DEFAULT '',
    tags JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(tags) = 'object'),
    native_details JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(native_details) = 'object'),
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aliyun_vpcs_keytuple ON aliyun_vpcs(cloud_account_id, region, network_id);
CREATE INDEX idx_aliyun_vpcs_live ON aliyun_vpcs(cloud_account_id, region, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_aliyun_vpcs_last_seen ON aliyun_vpcs(last_seen_at);
CREATE INDEX idx_aliyun_vpcs_native_ref ON aliyun_vpcs(cloud_account_id, region, native_ref);
CREATE INDEX idx_aliyun_vpcs_tags ON aliyun_vpcs USING GIN (tags);

CREATE TABLE aliyun_vswitches (
    id BIGSERIAL PRIMARY KEY,
    cloud_account_id BIGINT NOT NULL REFERENCES cloud_accounts(id),
    subnet_id TEXT NOT NULL,
    network_ref TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL,
    zone TEXT NOT NULL DEFAULT '',
    cidr_blocks JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(cidr_blocks) = 'array'),
    tags JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(tags) = 'object'),
    native_details JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(native_details) = 'object'),
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aliyun_vswitches_keytuple ON aliyun_vswitches(cloud_account_id, region, subnet_id);
CREATE INDEX idx_aliyun_vswitches_live ON aliyun_vswitches(cloud_account_id, region, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_aliyun_vswitches_last_seen ON aliyun_vswitches(last_seen_at);
CREATE INDEX idx_aliyun_vswitches_network_ref ON aliyun_vswitches(cloud_account_id, region, network_ref);
CREATE INDEX idx_aliyun_vswitches_tags ON aliyun_vswitches USING GIN (tags);

CREATE TABLE aliyun_databases (
    id BIGSERIAL PRIMARY KEY,
    cloud_account_id BIGINT NOT NULL REFERENCES cloud_accounts(id),
    database_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    product TEXT NOT NULL DEFAULT '',
    engine TEXT NOT NULL DEFAULT '',
    engine_version TEXT NOT NULL DEFAULT '',
    instance_class TEXT NOT NULL DEFAULT '',
    native_status TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL,
    zone TEXT NOT NULL DEFAULT '',
    endpoints JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(endpoints) = 'array'),
    storage_gb BIGINT,
    ha_mode TEXT,
    publicly_accessible BOOLEAN,
    tags JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(tags) = 'object'),
    native_details JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(native_details) = 'object'),
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aliyun_databases_keytuple ON aliyun_databases(cloud_account_id, region, database_id);
CREATE INDEX idx_aliyun_databases_live ON aliyun_databases(cloud_account_id, region, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_aliyun_databases_last_seen ON aliyun_databases(last_seen_at);
CREATE INDEX idx_aliyun_databases_tags ON aliyun_databases USING GIN (tags);

CREATE TABLE aliyun_security_groups (
    id BIGSERIAL PRIMARY KEY,
    cloud_account_id BIGINT NOT NULL REFERENCES cloud_accounts(id),
    firewall_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    network_ref TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    inbound_rules JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(inbound_rules) = 'array'),
    outbound_rules JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(outbound_rules) = 'array'),
    tags JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(tags) = 'object'),
    native_details JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(native_details) = 'object'),
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aliyun_security_groups_keytuple ON aliyun_security_groups(cloud_account_id, region, firewall_id);
CREATE INDEX idx_aliyun_security_groups_live ON aliyun_security_groups(cloud_account_id, region, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_aliyun_security_groups_last_seen ON aliyun_security_groups(last_seen_at);
CREATE INDEX idx_aliyun_security_groups_network_ref ON aliyun_security_groups(cloud_account_id, region, network_ref);
CREATE INDEX idx_aliyun_security_groups_tags ON aliyun_security_groups USING GIN (tags);

-- +goose Down
DROP TABLE aliyun_security_groups;
DROP TABLE aliyun_databases;
DROP TABLE aliyun_vswitches;
DROP TABLE aliyun_vpcs;
DROP TABLE aliyun_instances;
