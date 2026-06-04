-- Reverse of 000001_init_schema.up.sql. Dropped in reverse dependency order
-- so child tables go before the tables they reference.

DROP TABLE IF EXISTS user_progress;
DROP TABLE IF EXISTS hints;
DROP TABLE IF EXISTS levels;
DROP TABLE IF EXISTS tools;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
