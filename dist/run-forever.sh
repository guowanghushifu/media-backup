#!/usr/bin/env bash
set -u

script_path="${BASH_SOURCE[0]}"
while [ -L "$script_path" ]; do
  script_dir="$(cd -P "$(dirname "$script_path")" && pwd)"
  script_path="$(readlink "$script_path")"
  case "$script_path" in
    /*) ;;
    *) script_path="$script_dir/$script_path" ;;
  esac
done
script_dir="$(cd -P "$(dirname "$script_path")" && pwd)"

uname_m="${MEDIA_BACKUP_UNAME_M:-$(uname -m)}"
case "$uname_m" in
  x86_64|amd64)
    binary_name="media-backup-linux-amd64"
    ;;
  aarch64|arm64)
    binary_name="media-backup-linux-arm64"
    ;;
  *)
    echo "unsupported architecture: $uname_m" >&2
    exit 1
    ;;
esac

binary_path="$script_dir/$binary_name"
if [ ! -x "$binary_path" ]; then
  echo "required executable not found or not executable: $binary_path" >&2
  exit 1
fi

restart_delay="${MEDIA_BACKUP_RESTART_DELAY:-30}"
stopping=0
child_pid=""

stop_child() {
  stopping=1
  echo "[$(date '+%F %T')] stopping by user"
  if [ -n "$child_pid" ] && kill -0 "$child_pid" 2>/dev/null; then
    kill -TERM "$child_pid" 2>/dev/null || true
    wait "$child_pid" 2>/dev/null || true
  fi
  exit 0
}

trap stop_child INT TERM

while true; do
  echo "[$(date '+%F %T')] starting media-backup: $binary_path $*"

  "$binary_path" "$@" &
  child_pid=$!
  wait "$child_pid"
  code=$?
  child_pid=""

  echo "[$(date '+%F %T')] media-backup exited with code $code"

  if [ "$stopping" -eq 1 ]; then
    echo "[$(date '+%F %T')] manual stop, not restarting"
    exit 0
  fi

  if [ "$code" -eq 0 ]; then
    echo "[$(date '+%F %T')] normal exit, not restarting"
    exit 0
  fi

  if [ "$code" -eq 130 ] || [ "$code" -eq 143 ]; then
    echo "[$(date '+%F %T')] interrupted, not restarting"
    exit 0
  fi

  echo "[$(date '+%F %T')] restarting in ${restart_delay} seconds..."
  sleep "$restart_delay"
done
