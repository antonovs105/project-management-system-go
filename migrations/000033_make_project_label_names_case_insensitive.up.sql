CREATE UNIQUE INDEX idx_project_labels_unique_name_ci
    ON project_labels (project_id, lower(name));
