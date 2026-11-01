package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wangkayu/reltitle/pkg/config"
	"github.com/wangkayu/reltitle/pkg/parser"
)

// version is the build's release version, injected with
// -ldflags "-X main.version=<X.Y.Z>"; the source tree builds as 0.0.0-dev.
var version = "0.0.0-dev"

// lineOf is the version line of a build — "major.minor" — ignoring a leading
// "v" and any prerelease suffix. A consumer speaks one line: a patch version
// keeps the output shape, a new minor version is a different parser.
func lineOf(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if dash := strings.IndexByte(v, '-'); dash >= 0 {
		v = v[:dash]
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return v
	}
	return parts[0] + "." + parts[1]
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run parses one title and writes the result as JSON to stdout. The title comes from
// -title, then the first positional argument, then the first line of stdin.
// `serve` runs the line protocol instead (see serve.go).
// Exit codes: 0 success, 1 config or parse failure, 2 usage error or no title.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "serve" {
		return serveServe(version, stdin, stdout, stderr)
	}

	fs := flag.NewFlagSet("reltitle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configDir := fs.String("config", "", "config directory (default: the configs embedded in the binary)")
	titleArg := fs.String("title", "", "title to parse")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "reltitle %s\n", version)
		return 0
	}

	var (
		p   *parser.Parser
		err error
	)
	if *configDir == "" {
		p, err = parser.NewDefault()
	} else {
		var cfg *config.Config
		cfg, err = config.LoadAll(*configDir)
		if err == nil {
			p = parser.New(cfg)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}

	raw := strings.TrimSpace(*titleArg)
	if raw == "" {
		if fs.NArg() > 0 {
			raw = strings.TrimSpace(fs.Arg(0))
		}
	}
	if raw == "" {
		scanner := bufio.NewScanner(stdin)
		if scanner.Scan() {
			raw = strings.TrimSpace(scanner.Text())
		}
	}
	if raw == "" {
		fmt.Fprintln(stderr, "no title provided")
		return 2
	}

	res, err := p.Parse(raw)
	if err != nil {
		fmt.Fprintf(stderr, "parse: %v\n", err)
		return 1
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
	return 0
}
