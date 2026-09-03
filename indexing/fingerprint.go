package indexing

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// Fingerprint hashes the contents of the given
// files into a short hex string suitable for
// naming an index. Tools pass the files that pin
// their corpus, such as a toolchain file and a
// dependency manifest.
func Fingerprint(paths ...string) (string, error) {
	h := sha256.New()
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return "", err
		}
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return "", err
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}
