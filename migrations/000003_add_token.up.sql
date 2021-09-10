ALTER TABLE dl.projects
ADD COLUMN token uuid NOT NULL DEFAULT gen_random_uuid();
