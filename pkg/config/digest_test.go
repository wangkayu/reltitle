package config

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestBundleDigestIsStableAndPathSensitive(t *testing.T) {
	files := func(a, b string) fstest.MapFS {
		return fstest.MapFS{
			"one.yaml": {Data: []byte(a)},
			"two.yaml": {Data: []byte(b)},
		}
	}
	first, err := BundleDigest(files("alpha", "beta"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := BundleDigest(files("alpha", "beta"))
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatalf("digest is not deterministic: %q vs %q", first, again)
	}
	if !strings.HasPrefix(first, "lexicon:sha256:") || len(first) != len("lexicon:sha256:")+64 {
		t.Fatalf("digest shape = %q", first)
	}
	// Renaming a file changes the digest even when the bytes are the same: the
	// path is part of the bundle identity.
	renamed := fstest.MapFS{
		"one.yaml":   {Data: []byte("alpha")},
		"three.yaml": {Data: []byte("beta")},
	}
	moved, err := BundleDigest(renamed)
	if err != nil {
		t.Fatal(err)
	}
	if moved == first {
		t.Fatal("renaming a file left the digest unchanged")
	}
}

func TestBundleDigestRejectsAnEmptyBundle(t *testing.T) {
	if _, err := BundleDigest(fstest.MapFS{}); err == nil {
		t.Fatal("an empty bundle produced a digest")
	}
}
