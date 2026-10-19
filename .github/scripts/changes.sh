#!/bin/sh
# Decide which CI jobs a change needs.
#
# Reads changed paths, one per line, from stdin and prints
#   go=true|false
#   extension=true|false
#   javascript=true|false     (CodeQL's javascript-typescript analysis)
#
# Classification (the first matching rule wins for each path):
#   docs only, run nothing:
#     markdown in the repository root, LICENSE, .devcontainer/**,
#     extension/store/**, extension/**/*.md (none of these are in the zip)
#   shared, run everything:
#     Makefile, .gitattributes, .github/**,
#     internal/classify/rules.json, internal/classify/vectors_test.go
#   extension/**/*.go: Go as well as extension and javascript
#   extension (and javascript): extension/**
#   Go: **/*.go, go.mod, go.sum, cmd/**, internal/**
#   Go and javascript: web/**
# Any other path runs everything, and so does an empty list, so a new kind of
# file or an odd diff can never silently skip checks.
set -eu

go=false
ext=false
js=false
seen=false

while IFS= read -r p || [ -n "$p" ]; do
	[ -n "$p" ] || continue
	seen=true
	case $p in
	extension/store/* | extension/*.md | .devcontainer/*) continue ;;
	*/*) ;;
	*.md | LICENSE) continue ;;
	esac
	case $p in
	Makefile | .gitattributes | .github/* | internal/classify/rules.json | internal/classify/vectors_test.go)
		go=true
		ext=true
		js=true
		;;
	extension/*.go)
		go=true
		ext=true
		js=true
		;;
	extension/*)
		ext=true
		js=true
		;;
	web/*)
		go=true
		js=true
		;;
	*.go | go.mod | go.sum | cmd/* | internal/*) go=true ;;
	*)
		go=true
		ext=true
		js=true
		;;
	esac
done

if [ "$seen" = false ]; then
	go=true
	ext=true
	js=true
fi
printf 'go=%s\nextension=%s\njavascript=%s\n' "$go" "$ext" "$js"
