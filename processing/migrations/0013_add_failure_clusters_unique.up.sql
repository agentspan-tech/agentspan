ALTER TABLE failure_clusters
    ADD CONSTRAINT uq_failure_clusters_org_label UNIQUE (organization_id, label);
