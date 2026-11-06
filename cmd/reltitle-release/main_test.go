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

// writeNotes writes a well-formed release notes file and returns its path.
func writeNotes(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "notes.md")
	body := "## zh-CN\n\n- 修复解析\n\n## en\n\n- Fix parsing\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
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
		"--notes", writeNotes(t, dir),
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
		"--sequence", "1", "--notes", writeNotes(t, dir),
		"--artifact", "linux/amd64="+artifact, "--out", out); code != 0 {
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

// The counter comes from a document the line already published, written by
// whatever release tool was current then: v0.0.3 carries `notes` as zh_cn/en
// bullet lists, a shape this build no longer writes. A strict decode would
// refuse the very file the next release has to advance.
func TestNextSequenceReadsAPreviouslyPublishedManifest(t *testing.T) {
	const published = `{
  "schema_version": "rt-channel-1",
  "line": "0.0",
  "protocol": "rt-serve-1",
  "version": "0.0.3",
  "sequence": 7,
  "published_at": "2026-11-01T08:59:41Z",
  "notes": {"zh_cn": ["- 旧版说明"], "en": ["- an older note"]},
  "artifacts": {"linux/amd64": {"path": "0.0.3/reltitle-linux-amd64", "sha256": "00", "size": 1}},
  "release_url": "https://example.invalid/releases/tag/v0.0.3"
}`
	path := filepath.Join(t.TempDir(), "channel.json")
	if err := os.WriteFile(path, []byte(published), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runReleaseOutput(t, "next-sequence", "--manifest", path)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "8" {
		t.Fatalf("next sequence = %q, want 8", strings.TrimSpace(stdout))
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
		"--sequence", "3", "--notes", writeNotes(t, dir),
		"--artifact", "linux/amd64="+artifact, "--key-file", private, "--out", out); code != 0 {
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
		"--sequence", "3", "--notes", writeNotes(t, dir),
		"--artifact", "linux/amd64="+artifact, "--key-file", private, "--out", out); code != 0 {
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
		"--sequence", "3", "--notes", writeNotes(t, dir),
		"--artifact", "linux/amd64="+artifact, "--key-file", private, "--out", out); code != 0 {
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

// A stable release carries its own summary, one bullet list per language, in
// the same document as the version and the artifacts.
func TestChannelCarriesTheBilingualNotes(t *testing.T) {
	dir := t.TempDir()
	artifact := writeArtifact(t, dir, "binary", "payload")
	notesPath := filepath.Join(dir, "notes.md")
	body := "## zh-CN\n\n- 修复无修版识别\n- 新增常驻解析服务\n\n## en\n\n- Recognize the uncensored marker\n- Add the long-lived parse server\n"
	if err := os.WriteFile(notesPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "branch")
	if code, stderr := runRelease(t, "channel", "--version", "0.0.3", "--sequence", "1",
		"--notes", notesPath, "--artifact", "linux/amd64="+artifact, "--out", out); code != 0 {
		t.Fatalf("channel exit %d: %s", code, stderr)
	}
	manifestPath := filepath.Join(out, "lines", "0.0", "channel.json")
	document, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "- 修复无修版识别\n- 新增常驻解析服务"
	if document.Notes == nil || document.Notes.ZhCN != want {
		t.Fatalf("zh-CN notes = %+v, want %q", document.Notes, want)
	}
	if document.Notes.EN != "- Recognize the uncensored marker\n- Add the long-lived parse server" {
		t.Fatalf("en notes = %q", document.Notes.EN)
	}
	if code, stderr := runRelease(t, "verify", "--manifest", manifestPath); code != 0 {
		t.Fatalf("verify exit %d: %s", code, stderr)
	}
}

// The version shape decides whether notes belong: a stable release must
// describe itself, a test build must not pretend to.
func TestNotesFollowTheVersionShape(t *testing.T) {
	dir := t.TempDir()
	artifact := writeArtifact(t, dir, "binary", "payload")
	notesPath := writeNotes(t, dir)

	code, stderr := runRelease(t, "channel", "--version", "0.0.3", "--sequence", "1",
		"--artifact", "linux/amd64="+artifact, "--out", filepath.Join(dir, "stable"))
	if code == 0 || !strings.Contains(stderr, "--notes") {
		t.Fatalf("a stable release without notes exited %d: %s", code, stderr)
	}
	code, stderr = runRelease(t, "channel", "--version", "0.0.3-test.7", "--sequence", "1",
		"--notes", notesPath, "--artifact", "linux/amd64="+artifact, "--out", filepath.Join(dir, "test"))
	if code == 0 || !strings.Contains(stderr, "carries no --notes") {
		t.Fatalf("a test build with notes exited %d: %s", code, stderr)
	}
	out := filepath.Join(dir, "test")
	if code, stderr := runRelease(t, "channel", "--version", "0.0.3-test.7", "--sequence", "1",
		"--artifact", "linux/amd64="+artifact, "--out", out); code != 0 {
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
	if _, present := decoded["notes"]; present {
		t.Fatalf("a test build carries a notes field: %s", raw)
	}
	if code, stderr := runRelease(t, "verify", "--manifest", manifestPath); code != 0 {
		t.Fatalf("verify exit %d: %s", code, stderr)
	}
}

// verify is the reader's gate, so it re-checks the notes rule rather than
// trusting the writer: a stable document without notes and a test build with
// them are both refused.
func TestVerifyRefusesNotesOutsideTheVersionShape(t *testing.T) {
	dir := t.TempDir()
	artifact := writeArtifact(t, dir, "binary", "payload")
	stable := filepath.Join(dir, "stable")
	if code, stderr := runRelease(t, "channel", "--version", "0.0.3", "--sequence", "1",
		"--notes", writeNotes(t, dir), "--artifact", "linux/amd64="+artifact, "--out", stable); code != 0 {
		t.Fatalf("channel exit %d: %s", code, stderr)
	}
	testBuild := filepath.Join(dir, "test")
	if code, stderr := runRelease(t, "channel", "--version", "0.0.3-test.7", "--sequence", "1",
		"--artifact", "linux/amd64="+artifact, "--out", testBuild); code != 0 {
		t.Fatalf("channel exit %d: %s", code, stderr)
	}

	rewrite := func(manifestPath string, mutate func(map[string]any)) string {
		t.Helper()
		raw, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		mutate(document)
		updated, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "channel.json")
		if err := os.WriteFile(path, append(updated, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	stablePath := filepath.Join(stable, "lines", "0.0", "channel.json")
	testPath := filepath.Join(testBuild, "lines", "0.0", "channel.json")

	stripped := rewrite(stablePath, func(document map[string]any) { delete(document, "notes") })
	code, stderr := runRelease(t, "verify", "--manifest", stripped)
	if code == 0 || !strings.Contains(stderr, "carries no release notes") {
		t.Fatalf("a stable manifest without notes exited %d: %s", code, stderr)
	}
	annotated := rewrite(testPath, func(document map[string]any) {
		document["notes"] = map[string]any{"zh-CN": "- x", "en": "- y"}
	})
	code, stderr = runRelease(t, "verify", "--manifest", annotated)
	if code == 0 || !strings.Contains(stderr, "carries release notes") {
		t.Fatalf("a test build with notes exited %d: %s", code, stderr)
	}
	empty := rewrite(stablePath, func(document map[string]any) {
		document["notes"] = map[string]any{"zh-CN": " ", "en": ""}
	})
	if code, _ := runRelease(t, "verify", "--manifest", empty); code == 0 {
		t.Fatal("a manifest with empty notes verified")
	}
}

// The notes file layout is narrow on purpose: a mangled file fails the release
// instead of publishing a summary for one language only.
func TestReadNotesRejectsMalformedFiles(t *testing.T) {
	long := strings.Repeat("字", maxNoteLength+1)
	tooMany := "## zh-CN\n\n- a\n- b\n- c\n- d\n- e\n- f\n\n## en\n\n- a\n"
	cases := map[string]string{
		"missing en":              "## zh-CN\n\n- 一条\n",
		"missing zh-CN":           "## en\n\n- a note\n",
		"empty bullet":            "## zh-CN\n\n-\n\n## en\n\n- a\n",
		"unknown section":         "## zh-TW\n\n- 一條\n\n## en\n\n- a\n",
		"text before sections":    "hello\n\n## zh-CN\n\n- 一条\n\n## en\n\n- a\n",
		"repeated section":        "## zh-CN\n\n- 一条\n\n## zh-CN\n\n- 两条\n\n## en\n\n- a\n",
		"en before zh-CN":         "## en\n\n- a\n\n## zh-CN\n\n- 一条\n",
		"section without bullets": "## zh-CN\n\n## en\n\n- a\n",
		"too many bullets":        tooMany,
		"bullet too long":         "## zh-CN\n\n- " + long + "\n\n## en\n\n- a\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "notes.md")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := readNotes(path); err == nil {
				t.Fatalf("notes parsed: %q", body)
			}
		})
	}
}
