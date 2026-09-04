-- +goose Up
UPDATE permissions SET description='View objects in Darkphish' WHERE slug='view_objects';
UPDATE permissions SET description='Create and edit objects in Darkphish' WHERE slug='modify_objects';

-- +goose Down
UPDATE permissions SET description='View objects in Gophish' WHERE slug='view_objects';
UPDATE permissions SET description='Create and edit objects in Gophish' WHERE slug='modify_objects';
