// Command reltitle-release writes a channel manifest and stages the branch
// layout a release publishes. It is the only place that defines the document,
// so the writer and the verifier cannot drift. Signing is optional: a channel
// published from this repository is trusted through the repository's HTTPS
// origin, and a key is only used when one is deliberately provided.
//
//	reltitle-release keygen    --private key.txt [--public-out pub.txt]
//	reltitle-release channel   --version 0.0.3 --sequence 3
//	                           --artifact linux/amd64=dist/reltitle-linux-amd64 ...
//	                           --key-file key.txt|--key-env NAME --out branch/
//	reltitle-release verify    --manifest channel.json --public-key pub.txt
//	reltitle-release next-sequence [--manifest channel.json]
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// SchemaVersion is the only channel layout a consumer accepts.
	SchemaVersion = "rt-channel-1"
	// Protocol is the serve protocol the published binaries speak.
	Protocol = "rt-serve-1"
)

// manifest is one signed channel document. Field order is the canonical
// serialization order, so Signature stays last.
type manifest struct {
	SchemaVersion string              `json:"schema_version"`
	Line          string              `json:"line"`
	Protocol      string              `json:"protocol"`
	Version       string              `json:"version"`
	Sequence      int64               `json:"sequence"`
	PublishedAt   string              `json:"published_at"`
	Artifacts     map[string]artifact `json:"artifacts"`
	ReleaseURL    string              `json:"release_url,omitempty"`
	// Signature is optional: a channel published without a signing key omits
	// it, and a consumer from the same trust anchor (HTTPS to this repository)
	// accepts an unsigned document. When a key is used, the field carries the
	// Ed25519 signature over the document without it.
	Signature string `json:"signature,omitempty"`
}

// artifact is one platform's download: a path relative to the channel's
// package root, plus the digest and size the consumer rechecks.
type artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// signingCopy is the exact byte set signed: the document without its signature.
func (m manifest) signingCopy() ([]byte, error) {
	m.Signature = ""
	return json.Marshal(m)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: reltitle-release <keygen|channel|verify|next-sequence> [flags]")
		return 2
	}
	var err error
	switch args[0] {
	case "keygen":
		err = keygen(args[1:], stderr)
	case "channel":
		err = channel(args[1:], stderr)
	case "verify":
		err = verify(args[1:], stderr)
	case "next-sequence":
		err = nextSequence(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func keygen(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	private := fs.String("private", "", "path to write the base64 secret seed (mode 0600)")
	publicOut := fs.String("public-out", "", "path to write the base64 public key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *private == "" {
		return errors.New("keygen needs --private")
	}
	public, secret, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*private, []byte(base64.StdEncoding.EncodeToString(secret.Seed())+"\n"), 0o600); err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(public)
	if *publicOut != "" {
		if err := os.WriteFile(*publicOut, []byte(encoded+"\n"), 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintln(stderr, "public key:", encoded)
	fmt.Fprintln(stderr, "keep the private key out of the repository and out of logs")
	return nil
}

func channel(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("channel", flag.ContinueOnError)
	fs.SetOutput(stderr)
	line := fs.String("line", "", "version line (major.minor); defaults to the version's line")
	version := fs.String("version", "", "release version, e.g. 0.0.3")
	sequence := fs.Int64("sequence", 0, "publication counter for this channel")
	publishedAt := fs.String("published-at", "", "RFC3339 timestamp; defaults to now")
	releaseURL := fs.String("release-url", "", "human-facing release page")
	keyFile := fs.String("key-file", "", "file holding the base64 secret seed")
	keyEnv := fs.String("key-env", "", "environment variable holding the base64 secret seed")
	out := fs.String("out", "", "staged branch root; the manifest lands in <out>/lines/<line>/channel.json")
	var artifacts stringList
	fs.Var(&artifacts, "artifact", "platform artifact as <os>/<arch>=<path>; repeat for each platform")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" || *out == "" {
		return errors.New("channel needs --version and --out")
	}
	if *sequence < 1 {
		return errors.New("channel needs a positive --sequence")
	}
	lineValue := *line
	if lineValue == "" {
		lineValue = lineOf(*version)
	}
	if lineValue != lineOf(*version) {
		return fmt.Errorf("version %s does not belong to line %s", *version, lineValue)
	}
	if len(artifacts) == 0 {
		return errors.New("channel needs at least one --artifact")
	}
	var key []byte
	if *keyFile != "" || *keyEnv != "" {
		loaded, err := loadKey(*keyFile, *keyEnv)
		if err != nil {
			return err
		}
		key = loaded
	}
	stamp := *publishedAt
	if stamp == "" {
		stamp = time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	}

	document := manifest{
		SchemaVersion: SchemaVersion,
		Line:          lineValue,
		Protocol:      Protocol,
		Version:       strings.TrimPrefix(*version, "v"),
		Sequence:      *sequence,
		PublishedAt:   stamp,
		Artifacts:     map[string]artifact{},
		ReleaseURL:    *releaseURL,
	}
	targetDir := filepath.Join(*out, "lines", lineValue, document.Version)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	for _, spec := range artifacts {
		platform, source, ok := strings.Cut(spec, "=")
		if !ok || platform == "" || source == "" {
			return fmt.Errorf("artifact %q is not <os>/<arch>=<path>", spec)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		name := "reltitle-" + strings.ReplaceAll(platform, "/", "-")
		if err := os.WriteFile(filepath.Join(targetDir, name), data, 0o755); err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		document.Artifacts[platform] = artifact{
			Path:   filepath.ToSlash(filepath.Join(document.Version, name)),
			SHA256: hex.EncodeToString(sum[:]),
			Size:   int64(len(data)),
		}
	}
	if len(key) > 0 {
		signature, err := sign(document, key)
		if err != nil {
			return err
		}
		document.Signature = signature
	}
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(*out, "lines", lineValue, "channel.json")
	if err := os.WriteFile(manifestPath, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "wrote %s (%s, sequence %d)\n", manifestPath, document.Version, document.Sequence)
	return nil
}

func verify(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	manifestPath := fs.String("manifest", "", "path to channel.json")
	publicKeyPath := fs.String("public-key", "", "path to the base64 public key (only needed when the manifest is signed)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	document, err := readManifest(*manifestPath)
	if err != nil {
		return err
	}
	if *publicKeyPath != "" || document.Signature != "" {
		if *publicKeyPath == "" {
			return errors.New("the manifest is signed; verification needs --public-key")
		}
		publicKey, err := readPublicKey(*publicKeyPath)
		if err != nil {
			return err
		}
		if err := checkSignature(document, publicKey); err != nil {
			return err
		}
	}
	fmt.Fprintf(stderr, "verified %s (%s, sequence %d)\n", document.Version, document.Line, document.Sequence)
	return nil
}

func nextSequence(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("next-sequence", flag.ContinueOnError)
	fs.SetOutput(stderr)
	manifestPath := fs.String("manifest", "", "path to the channel's current channel.json (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	next := int64(1)
	if *manifestPath != "" {
		if _, err := os.Stat(*manifestPath); err == nil {
			document, err := readManifest(*manifestPath)
			if err != nil {
				return err
			}
			next = document.Sequence + 1
		}
	}
	fmt.Fprintln(stdout, next)
	return nil
}

func readManifest(path string) (manifest, error) {
	var document manifest
	if path == "" {
		return document, errors.New("a manifest path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return document, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return document, fmt.Errorf("parse manifest: %w", err)
	}
	if document.SchemaVersion != SchemaVersion {
		return document, fmt.Errorf("manifest schema %q is not %s", document.SchemaVersion, SchemaVersion)
	}
	if document.Version == "" || document.Line == "" || len(document.Artifacts) == 0 {
		return document, errors.New("manifest is missing version, line or artifacts")
	}
	return document, nil
}

func loadKey(keyFile, keyEnv string) (ed25519.PrivateKey, error) {
	encoded := ""
	switch {
	case keyFile != "" && keyEnv != "":
		return nil, errors.New("choose --key-file or --key-env, not both")
	case keyFile != "":
		raw, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, err
		}
		encoded = strings.TrimSpace(string(raw))
	case keyEnv != "":
		encoded = strings.TrimSpace(os.Getenv(keyEnv))
	default:
		return nil, errors.New("channel needs --key-file or --key-env")
	}
	if encoded == "" {
		return nil, errors.New("the signing key is empty")
	}
	seed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode signing key: %w", err)
	}
	switch len(seed) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(seed), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(seed), nil
	default:
		return nil, fmt.Errorf("signing key is %d bytes, want %d", len(seed), ed25519.SeedSize)
	}
}

func readPublicKey(path string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key is %d bytes, want %d", len(decoded), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(decoded), nil
}

func sign(document manifest, key ed25519.PrivateKey) (string, error) {
	canonical, err := document.signingCopy()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, canonical)), nil
}

func checkSignature(document manifest, publicKey ed25519.PublicKey) error {
	signature, err := base64.StdEncoding.DecodeString(document.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	canonical, err := document.signingCopy()
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, canonical, signature) {
		return errors.New("the manifest signature does not verify")
	}
	return nil
}

// lineOf is the version line of a version string.
func lineOf(version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if dash := strings.IndexByte(version, '-'); dash >= 0 {
		version = version[:dash]
	}
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

// stringList collects a repeated flag.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}
