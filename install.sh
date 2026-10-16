#!/usr/bin/env bash
#
# SubFlow one-click installer and manager.
#
# Install or update:
#   curl -fsSL https://raw.githubusercontent.com/MinimaxFlora/SubFlow/master/install.sh | bash
#
# Manage an existing installation:
#   bash install.sh update
#   bash install.sh status | start | stop | restart | log | uninstall
#
# The script installs the release binary to /usr/local/bin/subflow, keeps
# configuration in /etc/subflow/subflow.env, stores data in /var/lib/subflow
# and registers a systemd (or OpenRC) service. It is non-interactive when stdin
# is not a TTY.

set -euo pipefail

REPO="MinimaxFlora/SubFlow"
SERVICE_NAME="subflow"
BIN_NAME="subflow"

BIN_PATH="/usr/local/bin/subflow"
RUN_PATH="/usr/local/bin/subflow-run"
CONF_DIR="/etc/subflow"
ENV_FILE="${CONF_DIR}/${SERVICE_NAME}.env"
DATA_DIR="/var/lib/subflow"
LOG_FILE="/var/log/subflow.log"

SYSTEMD_UNIT="/etc/systemd/system/${SERVICE_NAME}.service"
OPENRC_UNIT="/etc/init.d/${SERVICE_NAME}"

SUBFLOW_HOST="${SUBFLOW_HOST:-0.0.0.0}"
SUBFLOW_PORT="${SUBFLOW_PORT:-8080}"
SUBFLOW_LOG_LEVEL="${SUBFLOW_LOG_LEVEL:-info}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PLAIN='\033[0m'

info() { printf '%b\n' "${BLUE}$*${PLAIN}"; }
ok() { printf '%b\n' "${GREEN}$*${PLAIN}"; }
warn() { printf '%b\n' "${YELLOW}$*${PLAIN}"; }
die() {
  printf '%b\n' "${RED}$*${PLAIN}" >&2
  exit 1
}

need_root() {
  if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    die "This command requires root. Re-run it with sudo."
  fi
}

have_systemd() { command -v systemctl > /dev/null 2>&1 && [[ -d /run/systemd/system ]]; }
have_openrc() { command -v rc-service > /dev/null 2>&1; }

detect_target() {
  local os machine
  os="$(uname -s)"
  machine="$(uname -m)"
  case "${os}" in
    Linux)
      case "${machine}" in
        x86_64 | amd64) echo "linux-amd64" ;;
        i386 | i486 | i586 | i686 | x86) echo "linux-386" ;;
        aarch64 | arm64 | armv8*) echo "linux-arm64" ;;
        armv5*) echo "linux-armv5" ;;
        armv6*) echo "linux-armv6" ;;
        armv7* | arm) echo "linux-armv7" ;;
        s390x) echo "linux-s390x" ;;
        *) die "Unsupported CPU architecture: ${machine}" ;;
      esac
      ;;
    Darwin)
      case "${machine}" in
        arm64 | aarch64) echo "darwin-arm64" ;;
        x86_64 | amd64) echo "darwin-amd64" ;;
        *) die "Unsupported CPU architecture: ${machine}" ;;
      esac
      ;;
    *) die "Unsupported operating system: ${os}" ;;
  esac
}

# Prefer the releases/latest redirect, which is not subject to the
# unauthenticated API rate limit, and fall back to the API.
resolve_latest_tag() {
  local url tag
  url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    --retry 5 --retry-delay 3 --connect-timeout 15 --max-time 60 \
    "https://github.com/${REPO}/releases/latest" 2> /dev/null || true)"
  tag="${url##*/tag/}"
  if [[ "${url}" == *"/tag/"* && -n "${tag}" && "${tag}" != "latest" ]]; then
    printf '%s' "${tag}"
    return 0
  fi
  curl -fsSL --retry 5 --retry-delay 3 --connect-timeout 15 --max-time 60 \
    "https://api.github.com/repos/${REPO}/releases/latest" 2> /dev/null \
    | grep -o '"tag_name":[[:space:]]*"[^"]*"' | head -n 1 \
    | sed -E 's/.*"([^"]+)".*/\1/'
}

TMP_DIR=""
cleanup() {
  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi
}
trap cleanup EXIT

download_binary() {
  local tag="$1" target="$2" asset base
  asset="subflow-${target}"
  base="https://github.com/${REPO}/releases/download/${tag}"
  TMP_DIR="$(mktemp -d)"

  info "Downloading ${asset}.tar.gz (${tag}) ..."
  curl -fL --retry 5 --retry-delay 3 --connect-timeout 15 \
    --speed-limit 1 --speed-time 300 \
    -o "${TMP_DIR}/${asset}.tar.gz" "${base}/${asset}.tar.gz" \
    || die "Failed to download the release archive. Verify network access to GitHub."
  [[ -s "${TMP_DIR}/${asset}.tar.gz" ]] || die "The downloaded archive is empty."

  verify_checksum "${base}/SHA256SUMS" "${TMP_DIR}/${asset}.tar.gz" "${asset}.tar.gz"

  tar -xzf "${TMP_DIR}/${asset}.tar.gz" -C "${TMP_DIR}" \
    || die "Failed to extract the release archive."
  [[ -f "${TMP_DIR}/${asset}" ]] || die "The archive did not contain the ${BIN_NAME} binary."
  install -d -m 0755 "$(dirname "${BIN_PATH}")"
  install -m 0755 "${TMP_DIR}/${asset}" "${BIN_PATH}"
}

verify_checksum() {
  local sums_url="$1" file="$2" name="$3" expected actual
  if ! curl -fsL --retry 3 --connect-timeout 15 -o "${TMP_DIR}/SHA256SUMS" "${sums_url}"; then
    warn "SHA256SUMS is unavailable for this release; skipping verification."
    return 0
  fi
  expected="$(awk -v f="${name}" '$2 == f { print $1 }' "${TMP_DIR}/SHA256SUMS" | head -n 1)"
  if [[ -z "${expected}" ]]; then
    warn "No checksum entry for ${name}; skipping verification."
    return 0
  fi
  actual="$(sha256sum "${file}" | awk '{ print $1 }')"
  if [[ "${expected}" != "${actual}" ]]; then
    die "Checksum mismatch for ${name}: expected ${expected}, got ${actual}."
  fi
  ok "Checksum verified."
}

write_env_file() {
  install -d -m 0755 "${CONF_DIR}" "${DATA_DIR}"
  if [[ -f "${ENV_FILE}" ]]; then
    info "Keeping the existing configuration at ${ENV_FILE}."
    return 0
  fi
  umask 077
  cat > "${ENV_FILE}" << EOF
# SubFlow configuration. See https://github.com/${REPO}
SUBFLOW_HOST=${SUBFLOW_HOST}
SUBFLOW_PORT=${SUBFLOW_PORT}
SUBFLOW_DATA_DIR=${DATA_DIR}
SUBFLOW_DB_PATH=${DATA_DIR}/subflow.db
SUBFLOW_LOG_LEVEL=${SUBFLOW_LOG_LEVEL}
EOF
  ok "Wrote ${ENV_FILE}."
}

write_run_wrapper() {
  cat > "${RUN_PATH}" << EOF
#!/bin/sh
# Generated by the SubFlow installer. Loads ${ENV_FILE} then starts SubFlow.
set -a
set +a
exec "${BIN_PATH}"
EOF
  chmod 0755 "${RUN_PATH}"
}

install_systemd_service() {
  cat > "${SYSTEMD_UNIT}" << EOF
[Unit]
Description=SubFlow subscription management server
Documentation=https://github.com/${REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${RUN_PATH}
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable "${SERVICE_NAME}" > /dev/null 2>&1 || true
}

install_openrc_service() {
  cat > "${OPENRC_UNIT}" << EOF
#!/sbin/openrc-run

name="subflow"
description="SubFlow subscription management server"
command="${RUN_PATH}"
command_user="root"
command_background="yes"
pidfile="/run/${SERVICE_NAME}.pid"
output_log="${LOG_FILE}"
error_log="${LOG_FILE}"

depend() {
	need net
	after firewall
}
EOF
  chmod 0755 "${OPENRC_UNIT}"
  rc-update add "${SERVICE_NAME}" default > /dev/null 2>&1 || true
}

install_service() {
  write_run_wrapper
  if have_systemd; then
    install_systemd_service
  elif have_openrc; then
    install_openrc_service
  else
    warn "No supported init system found; the binary was installed but no service was registered."
    warn "Start it manually with: ${RUN_PATH}"
    return 0
  fi
  SERVICE_INSTALLED=true
  service_restart
}

SERVICE_INSTALLED=false

service_start() {
  if have_systemd; then systemctl start "${SERVICE_NAME}"; else rc-service "${SERVICE_NAME}" start; fi
}
service_stop() {
  if have_systemd; then systemctl stop "${SERVICE_NAME}"; else rc-service "${SERVICE_NAME}" stop; fi
}
service_restart() {
  if have_systemd; then systemctl restart "${SERVICE_NAME}"; else rc-service "${SERVICE_NAME}" restart; fi
}
service_status() {
  if have_systemd; then systemctl status "${SERVICE_NAME}" --no-pager; else rc-service "${SERVICE_NAME}" status; fi
}
service_logs() {
  if have_systemd; then journalctl -u "${SERVICE_NAME}" -n 200 --no-pager; else tail -n 200 "${LOG_FILE}"; fi
}
service_enable() {
  if have_systemd; then systemctl enable "${SERVICE_NAME}"; else rc-update add "${SERVICE_NAME}" default; fi
}
service_disable() {
  if have_systemd; then systemctl disable "${SERVICE_NAME}"; else rc-update del "${SERVICE_NAME}" default; fi
}

print_summary() {
  local tag="$1" target="$2"
  echo
  if [[ "${SERVICE_INSTALLED}" == "true" ]]; then
    ok "SubFlow ${tag} (${target}) is installed and running."
  else
    ok "SubFlow ${tag} (${target}) is installed."
    warn "No service is registered; start it with ${RUN_PATH}"
  fi
  echo
  echo "  Configuration : ${ENV_FILE}"
  echo "  Data          : ${DATA_DIR}"
  echo "  Web interface : http://<server-ip>:${SUBFLOW_PORT}/"
  echo
  echo "  Manage with   : ${CTL_PATH_ALIAS}"
  echo "    start | stop | restart | status | log | update | uninstall"
  echo
}

CTL_PATH_ALIAS="/usr/local/bin/subflowctl"

install_ctl() {
  if [[ -f "$0" && "$0" != "${CTL_PATH_ALIAS}" ]]; then
    install -m 0755 "$0" "${CTL_PATH_ALIAS}"
  fi
}

cmd_install() {
  need_root
  local target tag
  target="$(detect_target)"
  tag="$(resolve_latest_tag)"
  [[ -n "${tag}" ]] || die "Failed to resolve the latest release. GitHub API may be rate limited; retry later."
  info "Installing SubFlow (${tag}) for ${target} ..."
  download_binary "${tag}" "${target}"
  write_env_file
  install_ctl
  install_service
  print_summary "${tag}" "${target}"
}

cmd_update() {
  need_root
  local target tag
  target="$(detect_target)"
  tag="$(resolve_latest_tag)"
  [[ -n "${tag}" ]] || die "Failed to resolve the latest release. GitHub API may be rate limited; retry later."
  info "Updating SubFlow to ${tag} for ${target} ..."
  download_binary "${tag}" "${target}"
  write_env_file
  install_ctl
  if [[ -f "${SYSTEMD_UNIT}" ]] || [[ -f "${OPENRC_UNIT}" ]]; then
    SERVICE_INSTALLED=true
    service_restart
  else
    install_service
  fi
  print_summary "${tag}" "${target}"
}

cmd_uninstall() {
  need_root
  if have_systemd && [[ -f "${SYSTEMD_UNIT}" ]]; then
    systemctl stop "${SERVICE_NAME}" > /dev/null 2>&1 || true
    systemctl disable "${SERVICE_NAME}" > /dev/null 2>&1 || true
    rm -f "${SYSTEMD_UNIT}"
    systemctl daemon-reload > /dev/null 2>&1 || true
  elif have_openrc && [[ -f "${OPENRC_UNIT}" ]]; then
    rc-service "${SERVICE_NAME}" stop > /dev/null 2>&1 || true
    rc-update del "${SERVICE_NAME}" default > /dev/null 2>&1 || true
    rm -f "${OPENRC_UNIT}"
  fi
  rm -f "${BIN_PATH}" "${RUN_PATH}" "${CTL_PATH_ALIAS}"
  ok "SubFlow has been uninstalled."
  info "Configuration (${ENV_FILE}) and data (${DATA_DIR}) were kept."
  info "Remove them manually if you no longer need them:"
  echo "  rm -rf ${CONF_DIR} ${DATA_DIR}"
}

usage() {
  cat << EOF
SubFlow installer and manager

Usage: bash install.sh [command]

Commands:
  install      Download the latest release and set up the service (default)
  update       Upgrade to the latest release, keeping configuration and data
  uninstall    Stop and remove the service and binary (keeps data)
  start        Start the service
  stop         Stop the service
  restart      Restart the service
  status       Show the service status
  log          Show the last 200 log lines
  enable       Enable autostart on boot
  disable      Disable autostart on boot
  version      Print the installed SubFlow version
  help         Show this help

Environment overrides: SUBFLOW_HOST, SUBFLOW_PORT, SUBFLOW_LOG_LEVEL
EOF
}

main() {
  local cmd="${1:-install}"
  case "${cmd}" in
    install) cmd_install ;;
    update | upgrade) cmd_update ;;
    uninstall | remove) cmd_uninstall ;;
    start) need_root; service_start ;;
    stop) need_root; service_stop ;;
    restart) need_root; service_restart ;;
    status) service_status ;;
    log | logs) service_logs ;;
    enable) need_root; service_enable ;;
    disable) need_root; service_disable ;;
    version)
      if [[ -x "${BIN_PATH}" ]]; then "${BIN_PATH}" --version; else die "SubFlow is not installed at ${BIN_PATH}."; fi
      ;;
    help | -h | --help) usage ;;
    *) die "Unknown command: ${cmd}. Run 'bash install.sh help'." ;;
  esac
}

main "$@"
