#!/bin/sh
# Work out which parts of the repository a workflow run touches and write the
# result to $GITHUB_OUTPUT (go, extension, javascript, codeql). Inputs come in
# as environment variables only: EVENT_NAME, BASE_SHA, HEAD_SHA, BEFORE_SHA.
#
# pull_request: files changed between the PR's merge base and its head.
# push:         files changed between the previous and the new tip. A new
#               branch (all-zero before), a force push that dropped the old
#               tip, or any git failure runs everything.
# anything else (schedule, workflow_dispatch, ...): run everything.
set -eu

zero=0000000000000000000000000000000000000000

list=
case ${EVENT_NAME:-} in
pull_request)
	[ -n "${BASE_SHA:-}" ] && [ -n "${HEAD_SHA:-}" ] &&
		list=$(git -c core.quotepath=off diff --no-renames --name-only "$BASE_SHA...$HEAD_SHA" 2>/dev/null) || list=
	;;
push)
	if [ -n "${BEFORE_SHA:-}" ] && [ "$BEFORE_SHA" != "$zero" ] && [ -n "${HEAD_SHA:-}" ] &&
		git cat-file -e "$BEFORE_SHA^{commit}" 2>/dev/null; then
		list=$(git -c core.quotepath=off diff --no-renames --name-only "$BEFORE_SHA" "$HEAD_SHA" 2>/dev/null) || list=
	fi
	;;
esac

here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# An empty list means "unknown", which changes.sh treats as run everything.
result=$(printf '%s\n' "$list" | sh "$here/changes.sh")

# CodeQL languages to analyse, as a JSON array for the job matrix.
codeql=
case $result in *go=true*) codeql='"go"' ;; esac
case $result in
*javascript=true*) codeql="${codeql:+$codeql,}\"javascript-typescript\"" ;;
esac

out=$(printf '%s\ncodeql=[%s]\n' "$result" "$codeql")
printf '%s\n' "$out"
if [ -n "${GITHUB_OUTPUT:-}" ]; then printf '%s\n' "$out" >>"$GITHUB_OUTPUT"; fi
