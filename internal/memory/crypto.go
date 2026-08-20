package memory

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

const encPrefix = "so:enc:v1:"

var (
	zstdOnce sync.Once
	zstdEnc  *zstd.Encoder
	zstdDec  *zstd.Decoder
)

func (s *Store) keyPath() string {
	return s.path + ".key"
}

func (s *Store) ensureKey() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, path := range []string{s.keyPath(), filepath.Join(dir, "memory.key")} {
		raw, err := os.ReadFile(path)
		if err == nil && len(raw) >= 32 {
			s.key = append([]byte(nil), raw[:32]...)
			return nil
		}
		if err == nil && len(raw) > 0 && len(raw) < 32 {
			return fmt.Errorf("memory key is truncated")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	buf := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return err
	}
	path := s.keyPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		raw, rerr := os.ReadFile(path)
		if rerr == nil && len(raw) >= 32 {
			s.key = append([]byte(nil), raw[:32]...)
			return nil
		}
		legacy := filepath.Join(dir, "memory.key")
		raw, rerr = os.ReadFile(legacy)
		if rerr == nil && len(raw) >= 32 {
			s.key = append([]byte(nil), raw[:32]...)
			return nil
		}
		return err
	}
	s.key = buf
	return nil
}

func (s *Store) sealText(ad, plain string) string {
	if s == nil || len(s.key) != 32 || plain == "" || strings.HasPrefix(plain, encPrefix) {
		return plain
	}
	compressed := compressBytes(plain)
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return plain
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return plain
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plain
	}
	out := gcm.Seal(nonce, nonce, compressed, []byte(ad))
	return encPrefix + base64.StdEncoding.EncodeToString(out)
}

func (s *Store) openText(ad, stored string) string {
	if s == nil || !strings.HasPrefix(stored, encPrefix) {
		return stored
	}
	if len(s.key) != 32 {
		return stored
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return stored
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return stored
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return stored
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return stored
	}
	plain, err := gcm.Open(nil, raw[:ns], raw[ns:], []byte(ad))
	if err != nil {
		return stored
	}
	return decompressBytes(plain)
}

func compressBytes(s string) []byte {
	zstdOnce.Do(func() {
		zstdEnc, _ = zstd.NewWriter(nil)
		zstdDec, _ = zstd.NewReader(nil)
	})
	if zstdEnc == nil {
		return []byte(s)
	}
	return zstdEnc.EncodeAll([]byte(s), nil)
}

func decompressBytes(b []byte) string {
	zstdOnce.Do(func() {
		zstdEnc, _ = zstd.NewWriter(nil)
		zstdDec, _ = zstd.NewReader(nil)
	})
	if zstdDec == nil {
		return string(b)
	}
	out, err := zstdDec.DecodeAll(b, nil)
	if err != nil {
		return string(b)
	}
	return string(out)
}
