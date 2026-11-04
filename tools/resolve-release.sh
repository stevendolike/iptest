#!/usr/bin/env bash
set -euo pipefail

tag=${1-}
selected=${2-}
mode=${3-workflow_dispatch}
# Validate literal inputs before asking Git to resolve them.
if [[ ! $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]]; then
    printf '%s\n' 'invalid release tag: expected vMAJOR.MINOR.PATCH[-prerelease]' >&2
    exit 1
fi
if [[ ! $selected =~ ^[0-9a-fA-F]{40}$ ]]; then
    printf '%s\n' 'invalid selected commit: expected a complete SHA' >&2
    exit 1
fi
if [[ $# -gt 3 || ! $mode =~ ^(workflow_dispatch|push|publish)$ ]]; then
    printf '%s\n' 'invalid release mode' >&2
    exit 1
fi
commit=$(git rev-parse --verify "${selected}^{commit}")

# Read the remote tag, including the peeled commit of an annotated tag.
# ls-remote status 2 means the exact ref is absent; network/auth errors do not.
read_remote_commit() {
    local refs status oid ref extra base='' peeled=''
    if refs=$(git ls-remote --exit-code origin "refs/tags/$tag" "refs/tags/${tag}^{}"); then
        while read -r oid ref extra; do
            if [[ ! $oid =~ ^[0-9a-fA-F]{40}$ || -n $extra ]]; then
                printf '%s\n' 'invalid remote tag response' >&2
                return 1
            fi
            case "$ref" in
                "refs/tags/$tag")
                    [[ -z $base ]] || return 1
                    base=$oid
                    ;;
                "refs/tags/${tag}^{}")
                    [[ -z $peeled ]] || return 1
                    peeled=$oid
                    ;;
                *) printf '%s\n' 'unexpected remote tag response' >&2; return 1 ;;
            esac
        done <<< "$refs"
        if [[ -z $base ]]; then
            printf '%s\n' 'remote tag response lacks its ref' >&2
            return 1
        fi
        remote_commit=${peeled:-$base}
    else
        status=$?
        if [[ $status -ne 2 ]]; then
            printf '%s\n' 'cannot read remote release tag' >&2
            return 1
        fi
        remote_commit=''
    fi
}

if [[ $mode == publish ]]; then
    # Claim a missing tag with a normal (never forced) Git ref creation. A
    # competing creation at another commit is rejected atomically by the remote.
    export GIT_TERMINAL_PROMPT=0
    read_remote_commit
    if [[ -n $remote_commit && $remote_commit != "$commit" ]]; then
        printf '%s\n' 'release tag commit mismatch' >&2
        exit 1
    fi
    if [[ -z $remote_commit ]]; then
        git push --porcelain origin "${commit}:refs/tags/${tag}" >&2
    fi
    # A push acceptance is not proof: verify the exact remote tag again.
    read_remote_commit
    if [[ $remote_commit != "$commit" ]]; then
        printf '%s\n' 'release tag commit mismatch after remote verification' >&2
        exit 1
    fi
elif [[ $mode == workflow_dispatch ]]; then
    # Historical tags cannot run the newer verification tools. Never rebuild or
    # overwrite them via a manual dispatch; the caller must choose an unused tag.
    if git show-ref --verify --quiet "refs/tags/$tag"; then
        printf '%s\n' 'existing release tag: choose a new tag for manual dispatch' >&2
        exit 1
    fi
else
    if ! git show-ref --verify --quiet "refs/tags/$tag"; then
        printf '%s\n' 'missing release tag for push event' >&2
        exit 1
    fi
    tag_commit=$(git rev-parse --verify "refs/tags/${tag}^{commit}")
    if [[ $tag_commit != "$commit" ]]; then
        printf '%s\n' 'release tag commit mismatch for push event' >&2
        exit 1
    fi
fi
# No successful outputs are emitted before all required checks pass.
printf 'tag=%s\ncommit=%s\n' "$tag" "$commit"
