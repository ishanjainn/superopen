package engine

import (
	"encoding/hex"

	"github.com/zeebo/xxh3"
)

// fileContentDigest is the hot-path XXH3 file identity hash.
func fileContentDigest(body []byte) string {
	sum := xxh3.Hash128(body).Bytes()
	return hex.EncodeToString(sum[:])
}
