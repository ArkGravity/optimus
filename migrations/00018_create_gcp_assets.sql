-- +goose Up
CREATE TABLE gcp_instances (
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
CREATE UNIQUE INDEX uq_gcp_instances_keytuple ON gcp_instances(cloud_account_id, zone, instance_id);
CREATE INDEX idx_gcp_instances_live ON gcp_instances(cloud_account_id, zone, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_gcp_instances_last_seen ON gcp_instances(last_seen_at);
CREATE INDEX idx_gcp_instances_location ON gcp_instances(cloud_account_id, region, zone);
CREATE INDEX idx_gcp_instances_tags ON gcp_instances USING GIN (tags);
CREATE INDEX idx_gcp_instances_network_interfaces ON gcp_instances USING GIN (network_interfaces);

CREATE TABLE gcp_networks (
    id BIGSERIAL PRIMARY KEY,
    cloud_account_id BIGINT NOT NULL REFERENCES cloud_accounts(id),
    network_id TEXT NOT NULL,
    native_ref TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL DEFAULT '' CHECK (region = ''),
    cidr_blocks JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(cidr_blocks) = 'array'),
    native_status TEXT NOT NULL DEFAULT '',
    tags JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(tags) = 'object'),
    native_details JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(native_details) = 'object'),
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_gcp_networks_keytuple ON gcp_networks(cloud_account_id, network_id);
CREATE INDEX idx_gcp_networks_live ON gcp_networks(cloud_account_id, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_gcp_networks_last_seen ON gcp_networks(last_seen_at);
CREATE INDEX idx_gcp_networks_native_ref ON gcp_networks(cloud_account_id, native_ref);
CREATE INDEX idx_gcp_networks_tags ON gcp_networks USING GIN (tags);

CREATE TABLE gcp_subnets (
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
CREATE UNIQUE INDEX uq_gcp_subnets_keytuple ON gcp_subnets(cloud_account_id, region, subnet_id);
CREATE INDEX idx_gcp_subnets_live ON gcp_subnets(cloud_account_id, region, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_gcp_subnets_last_seen ON gcp_subnets(last_seen_at);
CREATE INDEX idx_gcp_subnets_network_ref ON gcp_subnets(cloud_account_id, region, network_ref);
CREATE INDEX idx_gcp_subnets_tags ON gcp_subnets USING GIN (tags);

CREATE TABLE gcp_databases (
    id BIGSERIAL PRIMARY KEY,
    cloud_account_id BIGINT NOT NULL REFERENCES cloud_accounts(id),
    database_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    product TEXT NOT NULL DEFAULT 'cloud_sql',
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
CREATE UNIQUE INDEX uq_gcp_databases_keytuple ON gcp_databases(cloud_account_id, region, database_id);
CREATE INDEX idx_gcp_databases_live ON gcp_databases(cloud_account_id, region, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_gcp_databases_last_seen ON gcp_databases(last_seen_at);
CREATE INDEX idx_gcp_databases_tags ON gcp_databases USING GIN (tags);

CREATE TABLE gcp_firewalls (
    id BIGSERIAL PRIMARY KEY,
    cloud_account_id BIGINT NOT NULL REFERENCES cloud_accounts(id),
    firewall_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    network_ref TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL DEFAULT '' CHECK (region = ''),
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
CREATE UNIQUE INDEX uq_gcp_firewalls_keytuple ON gcp_firewalls(cloud_account_id, firewall_id);
CREATE INDEX idx_gcp_firewalls_live ON gcp_firewalls(cloud_account_id, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_gcp_firewalls_last_seen ON gcp_firewalls(last_seen_at);
CREATE INDEX idx_gcp_firewalls_network_ref ON gcp_firewalls(cloud_account_id, network_ref);
CREATE INDEX idx_gcp_firewalls_tags ON gcp_firewalls USING GIN (tags);

-- +goose Down
DROP TABLE gcp_firewalls;
DROP TABLE gcp_databases;
DROP TABLE gcp_subnets;
DROP TABLE gcp_networks;
DROP TABLE gcp_instances;
