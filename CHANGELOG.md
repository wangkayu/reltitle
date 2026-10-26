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
