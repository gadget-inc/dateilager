CREATE TYPE hash AS (h1 uuid, h2 uuid);

CREATE SCHEMA dl;

CREATE TABLE dl.objects (
    project       integer,
    start_version bigint,
    stop_version  bigint,
    path          text,
    hash          hash,
    mode          integer,
    size          integer,
    packed        boolean
);

CREATE UNIQUE INDEX objects_idx ON dl.objects
    (project, start_version, stop_version, path text_pattern_ops);

CREATE UNIQUE INDEX objects_latest_idx ON dl.objects
    (project, stop_version, path text_pattern_ops);

CREATE TABLE dl.contents (
    hash  hash  PRIMARY KEY,
    bytes bytea
);

CREATE TABLE dl.projects (
    id             integer PRIMARY KEY,
    latest_version bigint
);
