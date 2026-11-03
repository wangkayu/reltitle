# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html),
starting at `v0.0.1`.

Because reltitle is consumed as a library, a version number describes the
compatibility of its output. A PATCH release (`0.0.x`) adds dictionary entries
or fixes parsing defects without changing the output shape — field names,
attribute dimensions, and their value keys stay as they are — so consumers need
no code change. A MINOR release (`0.x.0`) changes the output shape (a field or
an attribute dimension is added, renamed, or removed), so consumers have to
adapt. `1.0.0` will be declared once the output shape is stable.

## [Unreleased]

### Fixed

- A four-digit year is never an episode-range bound. `<title> <year> - <ep>`
  (`Ranma ½ 2024 - 27`) no longer parses as the range `2024-27`, which left the
  video with no episode; the trailing episode is reported instead. The guard
  covers every range-candidate branch, including the one that reuses the year
  token after year extraction removed it. Real ranges (`1080-1085`,
  `01 ~ 20`) are unchanged.

## [0.0.3] - 2026-11-01

### Added

- `reltitle serve` runs a line protocol on stdin/stdout (`{"id","op","title"}` in,
  one JSON response per line out) for a long-lived consumer: `hello` reports the
  protocol (`rt-serve-1`), the build's version and line, and the dictionary
  digest; `vocabulary` lists every attribute dimension's canonical values; `parse`
  returns the same `ParseResult` the one-shot CLI prints. A bad request is
  answered with an error and the process stays up.
- `-version` prints the build's version, injected at build time with
  `-ldflags "-X main.version=<X.Y.Z>"`.
- `pkg/config.BundleDigest` computes the `lexicon:sha256:…` digest of a YAML
  bundle, so a consumer and a separately built binary can prove they read the
  same dictionary.
- `cmd/reltitle-release` writes the channel manifest (`rt-channel-1`) and stages
  the branch a release publishes; `scripts/publish-channel.sh` writes that branch
  as one fast-forward commit. The manifest can be signed with an Ed25519 key
  (`keygen`, `--key-file`/`--key-env`), but the published channel is unsigned:
  a consumer trusts the repository it reads over HTTPS, not a key file.

### Fixed

- `无修版` / `無修版` / `完全无修版` / `完全無修版` and `[Uncensored Ver.]` now
  emit `content_rating=UNCENSORED` instead of only a version note, matching the
  value `无修正` and `Uncensored` already produced. A consumer filtering on
  `content_rating` therefore sees both spellings alike.
- The Fujimoto Tatsuki 17-26 title knowledge now also triggers on the western
  name order `Tatsuki Fujimoto 17-26`. A release whose title suffix reads
  `Tatsuki Fujimoto 17-26 S01 | 01-08` previously had `17-26` read as a
  season/episode pair (`S17E26`); the title span is now protected and `01-08`
  is parsed as the collection's episode range.

## [0.0.2] - 2026-10-26

### Fixed

- The bracketed composite subtitle codes `JPSC` and `JPTC` are now recognized
  as `sub_lang` input tokens. Before, they were only produced as normalized
  output values, so a title or filename tag such as `[JPSC]` yielded no
  subtitle language.

## [0.0.1] - 2026-10-26

First published version.

### Added

- `reltitle` command-line tool: pass a release title with `-title`, as a
  positional argument, or on standard input, and it prints the complete parse
  result as indented JSON.
- `pkg/parser` as a library: build a parser from the embedded default
  configuration with `parser.NewDefault`, or from your own YAML directory with
  `config.LoadAll` and `parser.New`, then call `Parse`. A built parser is
  read-only and safe to share across goroutines.
- Title decomposition into release group (head and tail), main title, title
  aliases, and search hints, each value carrying its token span and the
  pipeline stage that produced it.
- Episode conclusions: single episode, episode list, episode range, season,
  season range, season plus episode, and specials, each on a numbering axis
  (seasonal, absolute, TV, OVA, SP), with the rejected candidates kept as
  alternates.
- Content classification into TV episode, movie, batch pack, or special, with a
  template identifier and a confidence value.
- Release attributes: source type, resolution, video and audio codecs, bit
  depth, container, HDR, CRC, platform and platform region, origin region,
  audio and subtitle flags and languages, audio track slots with channels,
  edition, version, release status, pack kind and pack profile, year, movie
  index, special index, contents, content rating, notes, and noise.
- The default configuration set — subtitle groups, title knowledge, canonical
  values, token synonyms, inference rules, template catalog, group route
  overrides, pack profiles, and the rest — as 13 YAML files under `configs/`,
  also embedded in the binary so no external files are needed at run time.
- A golden corpus test that pins the full parse output of several hundred real
  release titles, so a rule change shows exactly which titles moved.
