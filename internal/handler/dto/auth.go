package dto

import (
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
)

// LoginRequest is the body of POST /api/auth/login. Login is by user id only
// in the demo MVP.
type LoginRequest struct {
	UserID string `json:"userId"`
}

// UserResponse is the public representation of an authenticated user.
type UserResponse struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

// ToUserResponse maps a domain auth.User into an API response DTO.
func ToUserResponse(u auth.User) UserResponse {
	return UserResponse{
		ID:        u.ID,
		Role:      u.Role,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}
}
