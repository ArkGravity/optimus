-- +goose Up
ALTER TABLE credentials_kubeconfigs
    ADD COLUMN auth_type VARCHAR(32) NOT NULL DEFAULT 'kubeconfig',
    ADD COLUMN cloud_key_id BIGINT REFERENCES credentials_cloud_keys(id) ON DELETE RESTRICT,
    ADD COLUMN gke_project VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN gke_location VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN gke_cluster VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN gke_endpoint VARCHAR(16) NOT NULL DEFAULT '',
    ADD CONSTRAINT credentials_kubeconfigs_auth_shape CHECK (
        (auth_type = 'kubeconfig' AND cloud_key_id IS NULL AND gke_project = '' AND gke_location = '' AND gke_cluster = '' AND gke_endpoint = '') OR
        (auth_type = 'gke_service_account' AND cloud_key_id IS NOT NULL AND gke_project <> '' AND gke_location <> '' AND gke_cluster <> '' AND gke_endpoint IN ('default', 'public', 'private', 'dns'))
    );
CREATE INDEX idx_credentials_kubeconfigs_cloud_key ON credentials_kubeconfigs(cloud_key_id) WHERE cloud_key_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM credentials_kubeconfigs WHERE auth_type = 'gke_service_account') THEN
        RAISE EXCEPTION 'Remove GKE Kubernetes credentials before downgrading to schema19';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE credentials_kubeconfigs
    DROP CONSTRAINT credentials_kubeconfigs_auth_shape,
    DROP COLUMN auth_type,
    DROP COLUMN cloud_key_id,
    DROP COLUMN gke_project,
    DROP COLUMN gke_location,
    DROP COLUMN gke_cluster,
    DROP COLUMN gke_endpoint;
