-- Initial schema for CrypticQuest. Types/constraints transcribed 1:1 from Plan.md.
-- Note: foreign keys only take effect with PRAGMA foreign_keys = ON, set per
-- connection by the application (see internal/db).

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'player',
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sessions (
    token      TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMP NOT NULL
);

CREATE TABLE tools (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    type        TEXT NOT NULL,
    title       TEXT NOT NULL,
    description TEXT,
    content     TEXT NOT NULL
);

CREATE TABLE levels (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    order_index     INTEGER NOT NULL UNIQUE,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL,
    flag            TEXT NOT NULL,
    unlocks_tool_id INTEGER REFERENCES tools(id)
);

CREATE TABLE hints (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    level_id    INTEGER NOT NULL REFERENCES levels(id) ON DELETE CASCADE,
    order_index INTEGER NOT NULL,
    text        TEXT NOT NULL
);

CREATE TABLE user_progress (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    level_id  INTEGER NOT NULL REFERENCES levels(id) ON DELETE CASCADE,
    solved_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, level_id)
);
