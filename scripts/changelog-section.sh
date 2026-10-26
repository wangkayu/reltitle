#!/usr/bin/env bash
#
# Print the body of one CHANGELOG.md release section, without its heading.
#
#   scripts/changelog-section.sh 0.0.1      # the "v0.0.1" tag form works too
#
# The section runs from the "## [<version>]" heading up to the next "## "
# heading, and surrounding blank lines are trimmed. Exit status: 0 when the
# section exists and is not empty, 1 when the version has no (or an empty)
# section, 2 on a usage or setup error.
set -euo pipefail

usage() {
	printf 'usage: %s <version>\n' "$(basename "$0")" >&2
	printf 'example: %s 0.0.1   # extracts "## [0.0.1] - ..." from CHANGELOG.md\n' "$(basename "$0")" >&2
}

if [ "$#" -ne 1 ] || [ -z "${1-}" ]; then
	usage
	exit 2
fi

# Accept both "0.0.1" and the "v0.0.1" form used by git tags.
version="${1#v}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
changelog="${CHANGELOG_PATH:-$repo_root/CHANGELOG.md}"

if [ ! -f "$changelog" ]; then
	printf 'changelog-section: no such file: %s\n' "$changelog" >&2
	exit 2
fi

awk -v want="$version" -v changelog="$changelog" '
	BEGIN { prefix = "## [" want "]"; found = 0; n = 0 }
	substr($0, 1, 3) == "## " {
		if (found) exit
		rest = substr($0, length(prefix) + 1)
		if (index($0, prefix) == 1 && (rest == "" || rest ~ /^[ \t]/)) {
			found = 1
			next
		}
	}
	found { lines[++n] = $0 }
	END {
		if (!found) {
			printf "changelog-section: %s has no \"## [%s]\" section\n", changelog, want > "/dev/stderr"
			exit 1
		}
		while (n > 0 && lines[n] ~ /^[[:space:]]*$/) n--
		first = 1
		while (first <= n && lines[first] ~ /^[[:space:]]*$/) first++
		if (first > n) {
			printf "changelog-section: the \"## [%s]\" section in %s is empty\n", want, changelog > "/dev/stderr"
			exit 1
		}
		for (i = first; i <= n; i++) print lines[i]
	}
' "$changelog"
