#!/usr/bin/env bash
# Installs butaca-parse and `butaca serve` as launchd agents for the current
# user, so they start at login and restart when they die. Safe to re-run: an
# agent that is already loaded is replaced.
#
#   deploy/launchd/install.sh             build, install and start both
#   deploy/launchd/install.sh uninstall   stop and remove both
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
butaca_dir="$(cd "$here/../.." && pwd)"
agents="$HOME/Library/LaunchAgents"
log_dir="$HOME/Library/Logs/butaca"
labels=(com.butaca.parse com.butaca.serve)

unload() {
  launchctl bootout "gui/$UID/$1" 2>/dev/null || true
}

if [[ "${1:-}" == "uninstall" ]]; then
  for label in "${labels[@]}"; do
    unload "$label"
    rm -f "$agents/$label.plist"
  done
  echo "removed ${labels[*]}"
  exit 0
fi

uv="$(command -v uv)" || { echo "uv is not installed: brew install uv" >&2; exit 1; }
command -v ffprobe >/dev/null || echo "warning: no ffprobe on PATH; brew install ffmpeg" >&2

(cd "$butaca_dir" && go build -o butaca ./cmd/butaca)
mkdir -p "$agents" "$log_dir"

for label in "${labels[@]}"; do
  sed -e "s|@BUTACA_DIR@|$butaca_dir|g" \
      -e "s|@UV@|$uv|g" \
      -e "s|@LOG_DIR@|$log_dir|g" \
      "$here/$label.plist" > "$agents/$label.plist"
  plutil -lint -s "$agents/$label.plist"
  unload "$label"
  launchctl bootstrap "gui/$UID" "$agents/$label.plist"
  echo "started $label, logs in $log_dir"
done
