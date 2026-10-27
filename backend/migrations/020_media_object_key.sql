
-- Store references to images in MinIO.
-- Actual image files remain in object storage.

ALTER TABLE school_profile
ADD COLUMN logo_object_key TEXT;

ALTER TABLE users
ADD COLUMN profile_picture_object_key TEXT;
