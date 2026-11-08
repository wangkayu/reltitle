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

## [0.1.0] - 2026-11-08

### Changed

- `content_rating` reports one value per fact: `UNCENSORED`, `CENSORED` and
  `AGE_RESTRICTED`. The age-limit spellings `R18`, `R18+`, `r-rated`,
  `restricted` and `mature` now normalize to `AGE_RESTRICTED`; previously each
  produced its own value. `adult` is no longer classified (it also names a
  network), and `uncut` stays an `UNCUT` edition note rather than a rating.
- Resolution reports one of the eight canonical tiers. A `WxH` frame takes its
  exact height tier (`1440x1080` and `2600x1080` are `1080p`, `848x480` is
  `480p`), otherwise the larger of its width family and its nearest tier
  (`3840x1600` is `2160p`). Any other `Np` spelling takes its nearest tier
  (`2276p` is `2160p`, `810p` is `720p`). The dictionary's ad-hoc heights are
  no longer output values.
- `version` values are uppercase `V<n>` (`V2`, not `v2`), matching the
  dictionary's `V2`–`V5` entries.
- A generic `MPEG` tag reports `MPEG2`; `MPEG` is no longer a video codec value.
- `[HDR10+]` reports `HDR10_PLUS`; the bracketed form used to split into
  `hdr10` and a stray `+`, yielding `HDR10`.
- A `MULTI`/`MULTISUB` note reports `sub_lang=MULTI`. It used to be promoted to
  `sub_flag=soft`, which also produced `sub_lang=MULTI` through a hint; the
  note no longer adds `sub_flag=soft`.
- `BD`, `BDRip` and `BluRay` are promoted by their own spelling. `[BD]` reports
  `BD` only — it used to also report `BDRip` for the same token; `BDRIP`/`BD-RIP`
  report `BDRip` and `BluRay`/`BLU-RAY` report `BluRay`.
- `remux` reports `source_type=Remux` instead of a `remux` dimension of its
  own; `DVD Remux` reports both `Remux` and `DVD`.
- `REPACK`, `RESEED`, `CORRECTED`/`FIXED` and `PROPER` report `revision`
  instead of the ad-hoc `repack`, `reseed` and `corrected` dimensions.
- A code enumeration of subtitle languages reports each listed code as its own
  `sub_lang`. `[JPSC&JPTC]` used to report nothing and now reports `JPSC` and
  `JPTC`; a bare `简繁`/`繁简` block is recognized as `SC` and `TC`.
- `reltitle serve`'s `vocabulary` lists only the dimensions whose values are
  closed and enumerable, and for those lists every value a parse can report:
  the canonical values of a dimension `dict_canonical_values.yaml` defines, and
  the lexicon and promoted values of one it does not (`content_rating`,
  `edition`, `hdr`). Structural (`contents`, `pack_kind`, `pack_profile`),
  free-form (`note`, `title`, `group`) and open or code-synthesized dimensions
  (`version`, `audio_object`, `audio_channels`, `sub_format`,
  `release_status`) are not published. A conformance test parses the golden
  corpus and fails on any published-dimension value the vocabulary lacks.

### Added

- Subtitle file names report their language: `zh`/`zho`/`chi` as `ZH`; a Chinese tag with an explicit `Hans` script or a `CN`/`SG`/`MY` region as `SC`, and with `Hant` or `TW`/`HK`/`MO` as `TC`, so `zh-Hans`, `zh-CN`, `zh-SG`, `zh-Hans-CN` and `zh-TW`, `zh-HK`, `zh-MO` all resolve (a bare or unknown region stays the generic `ZH`); `ja`/`jp`/`jpn`/`japanese` as `JPN`, `en`/`eng`/`english`/`en-US` as `ENG`, `ko`/`korean` as `KOR`, and any other known language by its primary subtag with a script or region (`ja-JP`, `en-GB`); `track2`–`track5` as `ENG`/`TC`/`SC`/`JPN`. A bracket holding only these tags is claimed as subtitles before the audio pass, so it no longer becomes an audio language or a title alias; ToonsHub keeps its own routing. A script/region-qualified `zh` reports only its `SC`/`TC` precision, never an extra generic `ZH`.
- Subtitle file names report their format: `ass`, `srt`, `ssa`, `pgs`, `sup`, `vobsub` and `vtt` are `sub_format` values, so `chs.vtt` reports `SC` and `VTT`.
- `Dual.Audio` and `Dual-Audio` report `audio_flag=DUAL_AUDIO` outside a bracket
  as well as inside one; the dotted spelling used to fail on its separator, and
  the parenthesized `Dual-Audio,` form used to keep the trailing comma.
- `杜比全景声` reports `audio_object=ATMOS`, the same as `atmos`.
- `1080P60` reports `resolution=1080p` and `note=60FPS`, the same as
  `1080p@60fps`.
- `RMVB` reports `container=RMVB`.
- `Deluxe Edition` reports `edition=DELUXE`.
- `简体字幕` and `繁字` report `sub_lang=SC` and `sub_lang=TC`.
- Canonical values for the TV channels and services the lexicon recognizes
  (`BS-NTV`, `CS-EX2`, `MYTV`, `TBS`, `MX`, `TVER`, `TENCENT`, `HBO_MAX`,
  `ANIPLUS`, …), for the source types `CAMRip`, `DVDScr`, `PPV` and `R5`, for
  `VP9`, plus canonical `bit_depth`, `platform_region` and `edition` sections.
  Every value the parser reports for these dimensions is now listed.
- `NHK-G`, `NHKG` and `NHK1` are token synonyms for `NHK_ONE`.
- Note promotion names one rule per canonical value, so `vocabulary` can list
  every promoted value, `MULTI` is a subtitle language, and `TS`/`M2TS`,
  `UHD`/`UHDRIP`/`UHD_BDRIP` and the disc-variant editions each report their
  own value instead of sharing a token-derived one.
- A `revision` attribute dimension (`REPACK`, `RESEED`, `CORRECTED`, `PROPER`)
  and an `upscaled` attribute dimension (`UPSCALED`, `AI_UPSCALED`,
  `UPSCALED_4K`). An upscale-to-4K spelling also reports
  `resolution=2160p`, and `vocabulary` publishes both dimensions.
- `[HLG]` reports `hdr=HLG`.
- `DTS:X`, `DTS-X` and `DTS.X` report `audio_codec=DTS` with
  `audio_object=DTSX`.

### Fixed

- Erai-raws' `(CA)` tag reported `audio_lang=YUE`; it marks Chinese audio and
  now reports `CMN`. The raw tag code is no longer emitted as a
  `note` (`tag_code:CA`, `tag_code:JA`); `(JA)` still reports `JPN`.
- The Maruko group route added a bracketed airdate as a note even though the
  same date was already reported as a `date` attribute. The note is gone.
- A `WxH` frame in the bracket immediately before the first tech token
  (`[848x480][RMVB]`) still reports its resolution; bracket tokens used to make
  the token distance look too large.
- `r-rated` now matches as the token sequence `r` `-` `rated`; as a single token
  it never matched.
- A bracketed `DTS-HD MA` (`DTS-HD.MA`, `DTS HD MA`) reports one
  `audio_codec=DTSHDMA` fact instead of `DTS` plus a `MA` platform. A bare `MA`
  in a tech block is no longer the Movies Anywhere platform; that platform is
  only implied by the lexicon's `WEB-DL`/`WEBRip` spellings.
- `reltitle serve`'s `vocabulary` reported the lexicon's ad-hoc spellings
  (`1008p`, `1440p`, `1440x1080`, …) beside a canonical dimension's values,
  contradicting the "canonical values" contract a consumer builds options from.
  It now reports exactly the canonical values.

## [0.0.4] - 2026-11-06

### Added

- A `date` attribute dimension: a calendar date (`2020-04-14`, `2026.03.29`,
  `2020/04/14`, `2020_04_14`, `2020年4月14日`, `20200414`) is reported
  normalized as `YYYY-MM-DD` with source `date_token`, and its tokens are
  claimed. No existing dimension, field or value key changes; a consumer that
  does not use the new dimension needs no change. The date's year is still
  reported as a `year` attribute.

### Fixed

- A four-digit year is never an episode-range bound. `<title> <year> - <ep>`
  (`Ranma ½ 2024 - 27`) no longer parses as the range `2024-27`, which left the
  video with no episode; the trailing episode is reported instead. The guard
  covers every range-candidate branch, including the one that reuses the year
  token after year extraction removed it. Real ranges (`1080-1085`,
  `01 ~ 20`) are unchanged.
- A bracketed date is no longer read as a leftover month–day range: the year
  token used to be removed alone, and `[2020-04-14]` became `RANGE 4-14`.
  With the whole date claimed, `[2020-04-14]` yields no episode or range.

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
