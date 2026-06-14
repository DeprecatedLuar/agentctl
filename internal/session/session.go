package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// ResolvedSession contains the user and session identifiers
type ResolvedSession struct {
	UserID    string
	SessionID string
}

// NewSessionID generates a unique session ID with format: YYYYMMDD_HHMMSS_<6-char-hex>
func NewSessionID() string {
	now := time.Now()
	timestamp := now.Format("20060102_150405")

	// Generate 3 random bytes (6 hex chars)
	randomBytes := make([]byte, 3)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-based randomness if crypto/rand fails
		randomBytes = []byte{byte(now.UnixNano() & 0xFF), byte((now.UnixNano() >> 8) & 0xFF), byte((now.UnixNano() >> 16) & 0xFF)}
	}

	randomHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("%s_%s", timestamp, randomHex)
}
