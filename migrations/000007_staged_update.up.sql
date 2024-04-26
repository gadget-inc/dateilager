CREATE TABLE dl.staged_objects (
    project       bigint  NOT NULL,
    start_version bigint,
    stop_version  bigint,
    path          text    NOT NULL,
    hash          hash,
    mode          bigint,
    size          bigint,
    packed        boolean
);

CREATE UNIQUE INDEX staged_objects_idx ON dl.staged_objects
    (project, start_version, stop_version, path text_pattern_ops);

CREATE UNIQUE INDEX staged_objects_latest_idx ON dl.staged_objects
    (project, stop_version, path text_pattern_ops);

CREATE UNIQUE INDEX identical_staged_objects_idx ON dl.staged_objects
    (project, (stop_version IS NULL), path, hash, mode) WHERE stop_version IS NULL;
