-- Invert the tool<->level unlock relation so one level can unlock many tools.
-- Before: levels.unlocks_tool_id -> tools.id (a level unlocked at most one tool).
-- After:  tools.unlocks_at_level_id -> levels.id (a level unlocks N tools).
-- ON DELETE SET NULL: deleting a level un-assigns its tools instead of blocking.

ALTER TABLE tools ADD COLUMN unlocks_at_level_id INTEGER REFERENCES levels(id) ON DELETE SET NULL;

-- Carry existing assignments over: each tool inherits the level that used to point at it.
UPDATE tools SET unlocks_at_level_id = (
    SELECT l.id FROM levels l WHERE l.unlocks_tool_id = tools.id
);

ALTER TABLE levels DROP COLUMN unlocks_tool_id;
