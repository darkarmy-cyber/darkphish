-- +goose Up
CREATE TABLE license_coordination (id INTEGER PRIMARY KEY);
INSERT INTO license_coordination (id) VALUES (1);

-- +goose Down
DROP TABLE license_coordination;
