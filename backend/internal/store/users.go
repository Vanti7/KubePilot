package store

import (
	"context"

	"github.com/kubepilot/backend/internal/models"
	"gorm.io/gorm"
)

// ActiveUserRole returns the live role and active flag for a user ID. It
// satisfies middleware.ActiveUserRole — JWTAuth calls it on every request so
// a role/active change made via PATCH /auth/users/:id takes effect
// immediately instead of only once the presented token expires.
func (s *Store) ActiveUserRole(ctx context.Context, userID string) (string, bool, error) {
	var user models.User
	err := s.DB.WithContext(ctx).
		Select("role", "is_active").
		Where("id = ?", userID).
		First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	return user.Role, user.IsActive, nil
}
