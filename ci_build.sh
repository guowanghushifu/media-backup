#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-}"
GO_BIN="${GO_BIN:-go}"
DIST_DIR="${DIST_DIR:-dist}"
BUILD_TMP_DIR="${BUILD_TMP_DIR:-${DIST_DIR}/.tmp}"
INSTALL_SYSTEMD_SCRIPT="${INSTALL_SYSTEMD_SCRIPT:-dist/install-systemd-service.sh}"
RUN_FOREVER_SCRIPT="${RUN_FOREVER_SCRIPT:-dist/run-forever.sh}"
CONFIG_EXAMPLE="${CONFIG_EXAMPLE:-configs/config.example.yaml}"

if [[ -z "${VERSION}" ]]; then
  echo "VERSION is required, for example: VERSION=v0.3.0 ./ci_build.sh" >&2
  exit 1
fi

version_without_v="${VERSION#v}"
install_systemd_contents="$(cat "${INSTALL_SYSTEMD_SCRIPT}")"
run_forever_contents="$(cat "${RUN_FOREVER_SCRIPT}")"
rm -rf "${DIST_DIR}" "${BUILD_TMP_DIR}"
mkdir -p "${DIST_DIR}" "${BUILD_TMP_DIR}"

build_archive() {
  local arch="$1"
  local binary_name="media-backup-linux-${arch}"
  local stage_dir="${BUILD_TMP_DIR}/${arch}"
  local archive_path="${DIST_DIR}/media-backup_${version_without_v}_linux_${arch}.tar.gz"

  mkdir -p "${stage_dir}"
  CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" "${GO_BIN}" build -trimpath -ldflags='-s -w' -o "${stage_dir}/${binary_name}" ./cmd/media-backup
  mkdir -p "${stage_dir}/configs"
  cp "${CONFIG_EXAMPLE}" "${stage_dir}/configs/config.example.yaml"
  printf '%s\n' "${install_systemd_contents}" > "${stage_dir}/install-systemd-service.sh"
  printf '%s\n' "${run_forever_contents}" > "${stage_dir}/run-forever.sh"
  chmod +x "${stage_dir}/${binary_name}" "${stage_dir}/install-systemd-service.sh" "${stage_dir}/run-forever.sh"
  tar -C "${stage_dir}" -czf "${archive_path}" "${binary_name}" install-systemd-service.sh run-forever.sh configs/config.example.yaml
}

build_archive amd64
build_archive arm64
rm -rf "${BUILD_TMP_DIR}"
