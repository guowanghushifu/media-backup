#!/usr/bin/env bash
set -euo pipefail

service_name="media-backup.service"
unit_dir="${MEDIA_BACKUP_SYSTEMD_UNIT_DIR:-/etc/systemd/system}"
systemctl_bin="${MEDIA_BACKUP_SYSTEMCTL_BIN:-systemctl}"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
unit_path="${unit_dir}/${service_name}"

usage() {
  cat <<'EOF'
Usage: install-systemd-service.sh [-i|-u]

  -i    install the systemd service
  -u    uninstall the systemd service

No argument defaults to install.
EOF
}

require_root() {
  if [[ "${MEDIA_BACKUP_SKIP_ROOT_CHECK:-0}" == "1" ]]; then
    return
  fi

  if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    echo "please run as root, for example: sudo ./install-systemd-service.sh" >&2
    exit 1
  fi
}

detect_arch() {
  local raw_arch

  raw_arch="${MEDIA_BACKUP_UNAME_M:-$(uname -m)}"
  case "${raw_arch}" in
    x86_64|amd64)
      printf 'amd64\n'
      ;;
    aarch64|arm64)
      printf 'arm64\n'
      ;;
    *)
      echo "unsupported CPU architecture: ${raw_arch}" >&2
      exit 1
      ;;
  esac
}

find_binary() {
  local arch
  local binary_path

  arch="$(detect_arch)"
  binary_path="${script_dir}/media-backup-linux-${arch}"

  if [[ ! -f "${binary_path}" || ! -x "${binary_path}" ]]; then
    echo "expected executable not found for ${arch}: ${binary_path}" >&2
    exit 1
  fi

  printf '%s\n' "${binary_path}"
}

write_unit_file() {
  local binary_path="$1"
  local working_dir

  working_dir="$(dirname -- "${binary_path}")"
  mkdir -p "${unit_dir}"

  cat >"${unit_path}" <<EOF
[Unit]
Description=media-backup service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${working_dir}
ExecStart=${binary_path}
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
EOF
}

install_service() {
  local binary_path

  require_root

  if [[ -f "${unit_path}" ]]; then
    echo "${service_name} already installed at ${unit_path}"
    return
  fi

  binary_path="$(find_binary)"
  write_unit_file "${binary_path}"

  "${systemctl_bin}" daemon-reload
  "${systemctl_bin}" enable --now "${service_name}"

  echo "installed ${service_name} using ${binary_path}"
}

uninstall_service() {
  require_root

  if [[ ! -f "${unit_path}" ]]; then
    echo "${service_name} is not installed"
    return
  fi

  "${systemctl_bin}" disable --now "${service_name}"
  rm -f "${unit_path}"
  "${systemctl_bin}" daemon-reload

  echo "uninstalled ${service_name}"
}

action="${1:-}"
case "${action}" in
  "")
    install_service
    ;;
  -i)
    install_service
    ;;
  -u)
    uninstall_service
    ;;
  -h|--help)
    usage
    ;;
  *)
    usage >&2
    exit 1
    ;;
esac
