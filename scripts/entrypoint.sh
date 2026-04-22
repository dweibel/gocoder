#!/bin/sh
set -eu

# Clone repos if REPO_URLS is set (comma-separated list of git URLs)
if [ -n "${REPO_URLS:-}" ]; then
    IFS=','
    for url in $REPO_URLS; do
        # Derive directory name from repo URL (strip .git suffix, take last path segment)
        repo_name=$(basename "$url" .git)
        target="/workspace/$repo_name"
        if [ -d "$target/.git" ]; then
            echo "Repo $repo_name already cloned, reusing."
        else
            echo "Cloning $url into $target..."
            git clone "$url" "$target"
            echo "Clone of $repo_name complete."
        fi
    done
    unset IFS
fi

# Execute CMD (default: sleep infinity)
exec "$@"
