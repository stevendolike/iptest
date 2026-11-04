#!/usr/bin/env bash
set -euo pipefail

tag=${1-}
selected=${2-}
# Validate the literal tag before asking Git to resolve it.
if [[ ! $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]]; then
    printf '%s\n' 'invalid release tag: expected vMAJOR.MINOR.PATCH[-prerelease]' >&2
    exit 1
fi
if [[ ! $selected =~ ^[0-9a-fA-F]{40}$ ]]; then
    printf '%s\n' 'invalid selected commit: expected a complete SHA' >&2
    exit 1
fi

if git show-ref --verify --quiet "refs/tags/$tag"; then
    commit=$(git rev-parse --verify "refs/tags/${tag}^{commit}")
else
    commit=$(git rev-parse --verify "${selected}^{commit}")
fi
# Emit outputs only after successful resolution; never create or move a tag here.
printf 'tag=%s\ncommit=%s\n' "$tag" "$commit"
