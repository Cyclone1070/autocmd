package randutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const hexRatio = 2

// ShortID generates a cryptographically random hex string of the given length.
func ShortID(length int) string {
	bytes := make([]byte, length/hexRatio)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("failed to generate random ID: %v", err))
	}
	return hex.EncodeToString(bytes)
}
