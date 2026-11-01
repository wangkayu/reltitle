package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeArtifact(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runRelease(t *testing.T, args ...string) (code int, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, errOut.String()
}

func runReleaseOutput(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// The channel command is the only writer of the signed document: what it
// stages verifies, names the artifact it copied, and keeps the line.
func TestChannelRoundTrip(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "key.txt")
	public := filepath.Join(dir, "pub.txt")
	if code, stderr := runRelease(t, "keygen", "--private", private, "--public-out", public); code != 0 {
		t.Fatalf("keygen exit %d: %s", code, stderr)
	}
	amd64 := writeArtifact(t, dir, "reltitle-linux-amd64", "amd64-binary")
	arm64 := writeArtifact(t, dir, "reltitle-linux-arm64", "arm64-binary")
	out := filepath.Join(dir, "branch")
	code, stderr := runRelease(t, "channel",
		"--version", "0.0.3", "--sequence", "3",
		"--published-at", "2026-11-02T00:00:00Z",
		"--release-url", "https://github.com/wangkayu/reltitle/releases/tag/v0.0.3",
		"--artifact", "linux/amd64="+amd64,
		"--artifact", "linux/arm64="+arm64,
		"--key-file", private, "--out", out)
	if code != 0 {
		t.Fatalf("channel exit %d: %s", code, stderr)
	}
	manifestPath := filepath.Join(out, "lines", "0.0", "channel.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var document manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("manifest does not decode strictly: %v", err)
	}
	if document.SchemaVersion != SchemaVersion || document.Protocol != Protocol ||
		document.Line != "0.0" || document.Version != "0.0.3" || document.Sequence != 3 {
		t.Fatalf("manifest = %+v", document)
	}
	entry, ok := document.Artifacts["linux/amd64"]
	if !ok {
		t.Fatalf("manifest has no linux/amd64 artifact: %+v", document.Artifacts)
	}
	if entry.Path != "0.0.3/reltitle-linux-amd64" || entry.Size != int64(len("amd64-binary")) || len(entry.SHA256) != 64 {
		t.Fatalf("artifact = %+v", entry)
	}
	staged, err := os.ReadFile(filepath.Join(out, "lines", "0.0", entry.Path))
	if err != nil {
		t.Fatalf("staged artifact: %v", err)
	}
	if string(staged) != "amd64-binary" {
		t.Fatalf("staged artifact = %q", staged)
	}
	if code, stderr := runRelease(t, "verify", "--manifest", manifestPath, "--public-key", public); code != 0 {
		t.Fatalf("verify exit %d: %s", code, stderr)
	}
	code, stdout, stderr := runReleaseOutput(t, "next-sequence", "--manifest", manifestPath)
	if code != 0 {
		t.Fatalf("next-sequence exit %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "4" {
		t.Fatalf("next sequence = %q, want 4", strings.TrimSpace(stdout))
	}
}

// Without a signing key the document is published unsigned: the field is
// absent, and verification is structural only.
func TestChannelWithoutAKeyOmitsTheSignature(t *testing.T) {
	dir := t.TempDir()
	artifact := writeArtifact(t, dir, "binary", "payload")
	out := filepath.Join(dir, "branch")
	if code, stderr := runRelease(t, "channel", "--version", "0.0.3",
		"--sequence", "1", "--artifact", "linux/amd64="+artifact, "--out", out); code != 0 {
		t.Fatalf("channel exit %d: %s", code, stderr)
	}
	manifestPath := filepath.Join(out, "lines", "0.0", "channel.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, signed := decoded["signature"]; signed {
		t.Fatalf("an unsigned channel carries a signature field: %s", raw)
	}
	if code, stderr := runRelease(t, "verify", "--manifest", manifestPath); code != 0 {
		t.Fatalf("verify exit %d: %s", code, stderr)
	}
}

func TestNextSequenceStartsAtOneWithoutAManifest(t *testing.T) {
	code, stdout, stderr := runReleaseOutput(t, "next-sequence")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "1" {
		t.Fatalf("first sequence = %q, want 1", strings.TrimSpace(stdout))
	}
}

func TestVerifyRejectsATamperedManifest(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "key.txt")
	public := filepath.Join(dir, "pub.txt")
	if code, stderr := runRelease(t, "keygen", "--private", private, "--public-out", public); code != 0 {
		t.Fatalf("keygen exit %d: %s", code, stderr)
	}
	artifact := writeArtifact(t, dir, "binary", "payload")
	out := filepath.Join(dir, "branch")
	if code, stderr := runRelease(t, "channel", "--version", "0.0.3",
		"--sequence", "3", "--artifact", "linux/amd64="+artifact, "--key-file", private, "--out", out); code != 0 {
		t.Fatalf("channel exit %d: %s", code, stderr)
	}
	manifestPath := filepath.Join(out, "lines", "0.0", "channel.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(raw, []byte(`"sequence": 3`), []byte(`"sequence": 4`), 1)
	if bytes.Equal(tampered, raw) {
		t.Fatalf("manifest layout changed; cannot tamper: %s", raw)
	}
	tamperedPath := filepath.Join(dir, "tampered.json")
	if err := os.WriteFile(tamperedPath, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := runRelease(t, "verify", "--manifest", tamperedPath, "--public-key", public); code == 0 {
		t.Fatal("a tampered manifest verified")
	}
}

func TestVerifyRejectsAnUnknownField(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "key.txt")
	public := filepath.Join(dir, "pub.txt")
	if code, stderr := runRelease(t, "keygen", "--private", private, "--public-out", public); code != 0 {
		t.Fatalf("keygen exit %d: %s", code, stderr)
	}
	artifact := writeArtifact(t, dir, "binary", "payload")
	out := filepath.Join(dir, "branch")
	if code, stderr := runRelease(t, "channel", "--version", "0.0.3",
		"--sequence", "3", "--artifact", "linux/amd64="+artifact, "--key-file", private, "--out", out); code != 0 {
		t.Fatalf("channel exit %d: %s", code, stderr)
	}
	manifestPath := filepath.Join(out, "lines", "0.0", "channel.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	extra := bytes.Replace(raw, []byte(`{`), []byte(`{"surprise": 1,`), 1)
	extraPath := filepath.Join(dir, "extra.json")
	if err := os.WriteFile(extraPath, extra, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := runRelease(t, "verify", "--manifest", extraPath, "--public-key", public); code == 0 {
		t.Fatal("a manifest with an unknown field verified")
	}
}

func TestChannelRejectsAVersionOutsideTheLine(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "key.txt")
	if code, stderr := runRelease(t, "keygen", "--private", private); code != 0 {
		t.Fatalf("keygen exit %d: %s", code, stderr)
	}
	artifact := writeArtifact(t, dir, "binary", "payload")
	code, stderr := runRelease(t, "channel", "--line", "0.1", "--version", "0.0.3",
		"--sequence", "1", "--artifact", "linux/amd64="+artifact, "--key-file", private, "--out", filepath.Join(dir, "branch"))
	if code == 0 {
		t.Fatal("a version outside its declared line was accepted")
	}
	if !strings.Contains(stderr, "does not belong") {
		t.Fatalf("stderr = %q", stderr)
	}
}

// A manifest signed by one key never verifies under another: the consumer's
// baked key is the only trust root.
func TestVerifyRejectsAnotherKey(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "key.txt")
	if code, stderr := runRelease(t, "keygen", "--private", private); code != 0 {
		t.Fatalf("keygen exit %d: %s", code, stderr)
	}
	artifact := writeArtifact(t, dir, "binary", "payload")
	out := filepath.Join(dir, "branch")
	if code, stderr := runRelease(t, "channel", "--version", "0.0.3",
		"--sequence", "3", "--artifact", "linux/amd64="+artifact, "--key-file", private, "--out", out); code != 0 {
		t.Fatalf("channel exit %d: %s", code, stderr)
	}
	otherPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(dir, "other.pub")
	if err := os.WriteFile(otherPath, []byte(base64.StdEncoding.EncodeToString(otherPublic)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := runRelease(t, "verify", "--manifest", filepath.Join(out, "lines", "0.0", "channel.json"), "--public-key", otherPath); code == 0 {
		t.Fatal("a manifest verified under another key")
	}
}
