#!/bin/sh
# Tests for changes.sh. Run: sh .github/scripts/changes_test.sh
set -eu
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
fail=0

# check NAME GO EXT JS PATHS...   (PATHS are joined with newlines)
check() {
	name=$1 want="go=$2 extension=$3 javascript=$4"
	shift 4
	got=$(
		if [ $# -gt 0 ]; then printf '%s\n' "$@"; fi | sh "$here/changes.sh" | tr '\n' ' ' | sed 's/ $//'
	)
	if [ "$got" = "$want" ]; then
		echo "ok   $name"
	else
		echo "FAIL $name: got '$got' want '$want'"
		fail=1
	fi
}

check "README only" false false false README.md
check "root docs and license" false false false CONTRIBUTING.md SECURITY.md LICENSE
check "devcontainer only" false false false .devcontainer/devcontainer.json
check "extension src only" false true true extension/src/background.js
check "go file under extension counts as Go too" true true true extension/tools/gen.go
check "extension manifest" false true true extension/manifest.json
check "extension store only" false false false extension/store/listing.txt
check "extension readme only" false false false extension/README.md
check "nested extension markdown" false false false extension/docs/guide.md
check "rules.json is shared" true true true internal/classify/rules.json
check "vectors_test is shared" true true true internal/classify/vectors_test.go
check "cmd only" true false false cmd/history-timeline/main.go
check "internal go only" true false false internal/store/store.go
check "go.mod" true false false go.mod
check "go.sum" true false false go.sum
check "web only" true false true web/static/app.js
check "Makefile" true true true Makefile
check ".gitattributes" true true true .gitattributes
check "workflow" true true true .github/workflows/ci.yml
check "renovate config" true true true renovate.json
check "mix docs and go" true false false README.md cmd/x/main.go
check "mix extension and go" true true true extension/src/a.js cmd/x/main.go
check "mix docs and extension store" false false false README.md extension/store/a.png
check "markdown under internal counts as Go" true false false internal/foo/README.md
check "empty list runs everything" true true true
check "unknown file runs everything" true true true .golangci.yml
check "path with spaces" true false false "internal/my dir/a b.go"
check "docs path with spaces" false false false "extension/store/a b.png"
check "quoted unusual path runs everything" true true true '"internal/caf\303\251.go"'
check "unicode path" true false false "internal/café/é.go"
check "path with dollar and quote chars" false true true "extension/src/\$x'y.js"
check "blank lines ignored" false false false "" README.md ""

# detect.sh: a rename out of a Go file into a docs path must still run Go.
repo=$(mktemp -d)
(
	cd "$repo"
	git init -q
	git config user.email t@example.invalid
	git config user.name t
	mkdir internal
	printf 'package x\n\nfunc A() {}\nfunc B() {}\nfunc C() {}\n' >internal/x.go
	git add -A
	git commit -qm one
	git mv internal/x.go NOTES.md
	git commit -qm two
	got=$(EVENT_NAME=push BEFORE_SHA=$(git rev-parse HEAD~1) HEAD_SHA=$(git rev-parse HEAD) sh "$here/detect.sh" | tr '\n' ' ')
	case $got in
	*go=true*) echo "ok   rename of a Go file to markdown runs Go" ;;
	*) echo "FAIL rename of a Go file to markdown: $got"; exit 1 ;;
	esac
) || fail=1
rm -rf "$repo"

exit $fail
