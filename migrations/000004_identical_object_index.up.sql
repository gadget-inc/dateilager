CREATE UNIQUE INDEX identical_objects_idx ON dl.objects
    (project, (stop_version IS NULL), path, hash, mode) WHERE stop_version IS NULL;
