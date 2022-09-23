CREATE TABLE dl.cache_versions (
    version       bigserial  PRIMARY KEY,
    hashes        hash[]     NOT NULL
);