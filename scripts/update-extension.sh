#!/usr/bin/env bash
set -euo pipefail

if [[ "$(git branch --show-current)" != main || "${GIT_REFLOG_ACTION:-}" != "pull origin main" ]]; then
    exit 0
fi

npm ci
npm run package
code --install-extension zsh-language-server.vsix --force

printf '%s\n' 'Zsh Language Server updated. Reload VS Code to use it.'
