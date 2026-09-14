ALTER TABLE users
    ADD COLUMN display_name VARCHAR(128) NULL AFTER external_subject,
    ADD COLUMN avatar_url VARCHAR(2048) NULL AFTER display_name;
