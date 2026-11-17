#!/bin/bash
set -euo pipefail
export NVM_DIR=/var/lib/n-manager/nvm
export HOME=/var/lib/n-manager/deploy
cd "$HOME"
# shellcheck disable=SC1091
. "$NVM_DIR/nvm.sh"
action="$1"
arg="${2:-}"
case "$action" in
  install) nvm install "$arg" ;;
  uninstall) nvm uninstall "$arg" ;;
  default) nvm alias default "$arg" ;;
  npm-install) npm install -g -- "$arg" ;;
  npm-uninstall) npm uninstall -g -- "$arg" ;;
  *) echo "unknown action" >&2; exit 2 ;;
esac
