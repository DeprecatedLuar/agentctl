package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	// Session ID format
	sessionIDTimeFormat = "20060102_150405" // YYYYMMDD_HHMMSS
	sessionIDSeparator  = "_"
	randomBytesSize     = 3 // 3 bytes = 6 hex chars
)

// ResolvedSession contains the user and session identifiers
type ResolvedSession struct {
	UserID    string
	SessionID string
}

// NewSessionID generates a unique session ID with format: YYYYMMDD_HHMMSS_<6-char-hex>
func NewSessionID() string {
	now := time.Now()
	timestamp := now.Format(sessionIDTimeFormat)

	// Generate random bytes for unique suffix
	randomBytes := make([]byte, randomBytesSize)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-based randomness if crypto/rand fails
		randomBytes = []byte{byte(now.UnixNano() & 0xFF), byte((now.UnixNano() >> 8) & 0xFF), byte((now.UnixNano() >> 16) & 0xFF)}
	}

	randomHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("%s%s%s", timestamp, sessionIDSeparator, randomHex)
}
