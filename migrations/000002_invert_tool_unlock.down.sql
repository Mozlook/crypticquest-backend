-- Reverse of 000002: restore levels.unlocks_tool_id -> tools.id.
-- Note: this is lossy when a level unlocked more than one tool — only one tool
-- per level survives the round-trip (the model it reverts to cannot hold more).

ALTER TABLE levels ADD COLUMN unlocks_tool_id INTEGER REFERENCES tools(id);

UPDATE levels SET unlocks_tool_id = (
    SELECT t.id FROM tools t WHERE t.unlocks_at_level_id = levels.id
);

ALTER TABLE tools DROP COLUMN unlocks_at_level_id;
