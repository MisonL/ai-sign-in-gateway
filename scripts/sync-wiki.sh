#!/usr/bin/env bash
set -euo pipefail

root_dir="$(git rev-parse --show-toplevel)"
source_dir="${WIKI_SOURCE_DIR:-$root_dir/wiki}"
worktree="${WIKI_WORKTREE:-$root_dir/../$(basename "$root_dir").wiki}"
commit_message="${WIKI_COMMIT_MESSAGE:-docs: update wiki}"

if [[ ! -d "$source_dir" ]]; then
  echo "Wiki source directory not found: $source_dir" >&2
  exit 1
fi

remote="${WIKI_REMOTE:-}"
if [[ -z "$remote" ]]; then
  origin_url="$(git -C "$root_dir" remote get-url origin)"
  case "$origin_url" in
    git@github.com:*.git)
      remote="${origin_url%.git}.wiki.git"
      ;;
    https://github.com/*.git)
      remote="${origin_url%.git}.wiki.git"
      ;;
    https://github.com/*)
      remote="${origin_url}.wiki.git"
      ;;
    *)
      echo "Cannot infer GitHub wiki remote from origin: $origin_url" >&2
      echo "Set WIKI_REMOTE=git@github.com:owner/repo.wiki.git and retry." >&2
      exit 1
      ;;
  esac
fi

if [[ ! -d "$worktree/.git" ]]; then
  git clone "$remote" "$worktree"
fi

target_remote="$(git -C "$worktree" remote get-url origin)"
if [[ "$target_remote" != *".wiki.git"* && "$target_remote" != *".wiki" ]]; then
  echo "Refusing to sync into a non-wiki git repository: $target_remote" >&2
  exit 1
fi

find "$worktree" -mindepth 1 -maxdepth 1 ! -name ".git" -exec rm -rf {} +
rsync -a "$source_dir"/ "$worktree"/

git -C "$worktree" add -A

if git -C "$worktree" diff --cached --quiet; then
  current_branch="$(git -C "$worktree" branch --show-current)"
  if [[ -z "$current_branch" ]]; then
    current_branch="master"
  fi
  if git -C "$worktree" rev-parse --abbrev-ref --symbolic-full-name "@{upstream}" >/dev/null 2>&1; then
    if [[ "$(git -C "$worktree" rev-list --count "@{upstream}..HEAD")" -gt 0 ]]; then
      git -C "$worktree" push
    else
      echo "No wiki changes to publish."
    fi
  elif git -C "$worktree" rev-parse --verify HEAD >/dev/null 2>&1; then
    git -C "$worktree" push -u origin "$current_branch"
  else
    echo "No wiki changes to publish."
  fi
  exit 0
fi

git -C "$worktree" commit -m "$commit_message"
git -C "$worktree" push
