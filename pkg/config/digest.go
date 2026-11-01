package config

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
)

// BundleDigest identifies the exact YAML bundle a build carries, as
// "lexicon:sha256:<hex>" over every YAML file at the root of fsys in sorted
// path order, each hashed as path\0len\0bytes.
//
// The digest is how a consumer tells whether two builds — the one linked into
// the program and the one running as a separate binary — read the same
// dictionary. It is defined here, next to the loader, so every producer
// computes it the same way.
func BundleDigest(fsys fs.FS) (string, error) {
	paths, err := fs.Glob(fsys, "*.yaml")
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("config bundle is empty")
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return "", err
		}
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(data)))
		hash.Write([]byte(path))
		hash.Write([]byte{0})
		hash.Write(length[:])
		hash.Write([]byte{0})
		hash.Write(data)
	}
	return "lexicon:sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
