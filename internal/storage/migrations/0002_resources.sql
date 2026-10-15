-- Sub-Store compatible resources.
--
-- The upstream API is a name-keyed document store: subscriptions, collections,
-- files, artifacts, shares and settings are all "documents" addressed by name.
-- Keeping them in one table mirrors that model and keeps the REST layer thin.

CREATE TABLE resources (
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    data       TEXT NOT NULL,
    sort       INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (kind, name)
);

CREATE INDEX idx_resources_kind_sort ON resources (kind, sort, name);
