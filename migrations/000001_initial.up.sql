CREATE TYPE hash AS (h1 uuid, h2 uuid);

CREATE SCHEMA dl;

CREATE TABLE dl.projects (
    id             integer PRIMARY KEY,
    latest_version bigint  NOT NULL
);

CREATE TABLE dl.objects (
    project       integer NOT NULL,
    start_version bigint  NOT NULL,
    stop_version  bigint,
    path          text    NOT NULL,
    hash          hash    NOT NULL,
    mode          integer NOT NULL,
    size          bigint  NOT NULL,
    packed        boolean NOT NULL
);

CREATE UNIQUE INDEX objects_idx ON dl.objects
    (project, start_version, stop_version, path text_pattern_ops);

CREATE UNIQUE INDEX objects_latest_idx ON dl.objects
    (project, stop_version, path text_pattern_ops);

CREATE TABLE dl.contents (
    hash      hash  PRIMARY KEY,
    bytes     bytea NOT NULL,
    names_tar bytea
);
