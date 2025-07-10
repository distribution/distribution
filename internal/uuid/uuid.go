package uuid

import (
	"github.com/google/uuid"
)

// Returns a new V7 UUID string. V7 UUIDs are time-ordered for better database performance.
// Panics on error to maintain compatibility with google/uuid's NewString() method.
func NewString() string {
	return uuid.Must(uuid.NewV7()).String()
}
