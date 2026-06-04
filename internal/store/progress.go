package store

import "fmt"

// CurrentLevel returns the player's current level number: the ordinal position
// of the level they are working on. With strictly linear play that is the count
// of solved levels plus one, so a fresh account (no progress) is on level 1.
//
// This is the player-facing ordinal (1, 2, 3 ...), deliberately independent of
// the gapped order_index values used internally only for ordering.
func (s *Store) CurrentLevel(userID int64) (int, error) {
	var solved int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM user_progress WHERE user_id = ?`,
		userID,
	).Scan(&solved); err != nil {
		return 0, fmt.Errorf("current level: %w", err)
	}
	return solved + 1, nil
}
