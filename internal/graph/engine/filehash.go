package engine

import (
	"encoding/hex"

	"github.com/zeebo/xxh3"
)

// fileContentDigest is the hot-path file identity hash. Stored in file_hashes.sha256
// (column name kept for schema compatibility). XXH3 for fast content hashing.
func fileContentDigest(body []byte) string {
	sum := xxh3.Hash128(body).Bytes()
	return hex.EncodeToString(sum[:])
}
