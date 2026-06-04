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

// IsLevelAccessible reports whether the level with the given order_index is
// unlocked for the user. A level is accessible when its rank in the ordered
// list — the count of levels with order_index <= it — is at most the player's
// current level. With strict linearity that is every solved level plus the next
// unsolved one. Works regardless of the gaps in order_index (10, 20, 30 ...).
func (s *Store) IsLevelAccessible(userID int64, orderIndex int) (bool, error) {
	current, err := s.CurrentLevel(userID)
	if err != nil {
		return false, err
	}
	var rank int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM levels WHERE order_index <= ?`,
		orderIndex,
	).Scan(&rank); err != nil {
		return false, fmt.Errorf("level rank: %w", err)
	}
	return rank <= current, nil
}
