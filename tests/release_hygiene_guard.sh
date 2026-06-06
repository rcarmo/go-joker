#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

version=$(sed -n 's/^const VERSION = "\(v[^"]*\)"/\1/p' core/runtime/version.go)
if [ -z "$version" ]; then
  echo "could not parse core/runtime/version.go VERSION" >&2
  exit 1
fi

notes="docs/RELEASE_NOTES_${version}.md"
if [ ! -f "$notes" ]; then
  echo "missing release notes for ${version}: ${notes}" >&2
  exit 1
fi

if ! grep -q "This fork is ${version}" README.md; then
  echo "README.md does not advertise ${version}" >&2
  exit 1
fi

if ! grep -q "${notes}" README.md; then
  echo "README.md does not link ${notes}" >&2
  exit 1
fi

if ! grep -q "Release Notes .* ${version}" "$notes"; then
  echo "${notes} does not appear to be titled for ${version}" >&2
  exit 1
fi

if [ ! -f docs/RELEASE_CHECKLIST.md ]; then
  echo "missing docs/RELEASE_CHECKLIST.md" >&2
  exit 1
fi

# If the tag exists locally, warn (but do not fail) when HEAD has moved past it.
if git rev-parse -q --verify "refs/tags/${version}" >/dev/null; then
  if ! git merge-base --is-ancestor "${version}" HEAD; then
    echo "warning: ${version} tag is not an ancestor of HEAD" >&2
  fi
  ahead=$(git rev-list --count "${version}..HEAD" || echo 0)
  if [ "${ahead}" != "0" ]; then
    echo "warning: HEAD is ${ahead} commit(s) ahead of ${version}; decide whether to tag a new patch release" >&2
  fi
fi

echo "release hygiene guard passed for ${version}"
