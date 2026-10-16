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
# and registers a systemd (or OpenRC) service.
#
# When a terminal is available the installer asks how SubFlow should be reached:
# directly by IP and port, or by a domain name with automatic HTTPS (which
# installs and configures Caddy as a reverse proxy). Set SUBFLOW_ACCESS_MODE=ip
# or SUBFLOW_ACCESS_MODE=domain to skip the prompt for unattended installs.
#
# Environment overrides: SUBFLOW_HOST, SUBFLOW_PORT, SUBFLOW_LOG_LEVEL,
# SUBFLOW_ACCESS_MODE, SUBFLOW_DOMAIN, SUBFLOW_ACME_EMAIL.

set -euo pipefail

REPO="MinimaxFlora/SubFlow"
SERVICE_NAME="subflow"
BIN_NAME="subflow"

BIN_PATH="/usr/local/bin/subflow"
RUN_PATH="/usr/local/bin/subflow-run"
CONF_DIR="/etc/subflow"
ENV_FILE="${CONF_DIR}/${SERVICE_NAME}.env"
INSTALL_CONF="${CONF_DIR}/install.conf"
DATA_DIR="/var/lib/subflow"
LOG_FILE="/var/log/subflow.log"

CADDYFILE="/etc/caddy/Caddyfile"
CADDY_SERVICE="caddy"

SYSTEMD_UNIT="/etc/systemd/system/${SERVICE_NAME}.service"
OPENRC_UNIT="/etc/init.d/${SERVICE_NAME}"

SUBFLOW_HOST="${SUBFLOW_HOST:-}"
SUBFLOW_PORT="${SUBFLOW_PORT:-8080}"
SUBFLOW_LOG_LEVEL="${SUBFLOW_LOG_LEVEL:-info}"

# Access mode: "ip" exposes SubFlow directly on SUBFLOW_PORT, "domain" puts
# Caddy in front of it and serves HTTPS on ports 80/443.
SUBFLOW_ACCESS_MODE="${SUBFLOW_ACCESS_MODE:-}"
SUBFLOW_DOMAIN="${SUBFLOW_DOMAIN:-}"
SUBFLOW_ACME_EMAIL="${SUBFLOW_ACME_EMAIL:-}"
ACCESS_MODE=""
DOMAIN=""

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

# A controlling terminal lets us prompt even when the script itself is piped
# through a shell (for example `curl ... | bash`), where stdin is the script.
is_interactive() {
  [[ -t 0 ]] || { [[ -r /dev/tty ]] && [[ -w /dev/tty ]]; }
}

prompt_line() {
  local __prompt="$1" __var="$2" __line=""
  if [[ -r /dev/tty ]]; then
    IFS= read -r -p "${__prompt}" __line < /dev/tty || __line=""
  elif [[ -t 0 ]]; then
    IFS= read -r -p "${__prompt}" __line || __line=""
  fi
  printf -v "${__var}" '%s' "${__line}"
}

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
  # In domain mode SubFlow sits behind Caddy, so bind it to the loopback
  # interface unless the operator explicitly chose a bind address.
  local host="${SUBFLOW_HOST}"
  if [[ -z "${host}" ]]; then
    if [[ "${ACCESS_MODE}" == "domain" ]]; then host="127.0.0.1"; else host="0.0.0.0"; fi
  fi
  umask 077
  cat > "${ENV_FILE}" << EOF
# SubFlow configuration. See https://github.com/${REPO}
SUBFLOW_HOST=${host}
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

# --- Access mode (direct IP vs. domain with Caddy) -------------------------

conf_get() {
  local key="$1"
  [[ -f "${INSTALL_CONF}" ]] || return 0
  sed -n -E "s/^${key}=(.*)$/\1/p" "${INSTALL_CONF}" | head -n 1
}

save_install_conf() {
  install -d -m 0755 "${CONF_DIR}"
  umask 077
  cat > "${INSTALL_CONF}" << EOF
# SubFlow installer state. Generated automatically; do not edit by hand.
ACCESS_MODE=${ACCESS_MODE}
DOMAIN=${DOMAIN}
ACME_EMAIL=${SUBFLOW_ACME_EMAIL}
EOF
}

normalize_domain() {
  local d="$1"
  d="${d#http://}"
  d="${d#https://}"
  d="${d%%/*}"
  d="${d%%:*}"
  printf '%s' "${d,,}"
}

valid_domain() {
  [[ "$1" =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$ ]]
}

prompt_access_mode() {
  local answer
  echo
  info "How should SubFlow be accessed?"
  echo "  1) Directly by IP address and port (default)"
  echo "  2) By a domain name with automatic HTTPS (installs Caddy)"
  while true; do
    prompt_line "Select [1/2]: " answer
    case "${answer}" in
      "" | 1) ACCESS_MODE="ip"; return 0 ;;
      2) ACCESS_MODE="domain"; return 0 ;;
      *) warn "Please enter 1 or 2." ;;
    esac
  done
}

prompt_domain() {
  local answer
  while true; do
    prompt_line "Domain name to serve (for example subflow.example.com): " answer
    answer="$(normalize_domain "${answer}")"
    if valid_domain "${answer}"; then
      DOMAIN="${answer}"
      return 0
    fi
    warn "That does not look like a valid domain name."
  done
}

# Resolve the access mode from environment, saved state, or an interactive
# prompt, and validate the associated domain when needed.
resolve_access_mode() {
  if [[ -n "${SUBFLOW_ACCESS_MODE}" ]]; then
    ACCESS_MODE="${SUBFLOW_ACCESS_MODE}"
  elif [[ -z "${SUBFLOW_DOMAIN}" ]]; then
    ACCESS_MODE="$(conf_get ACCESS_MODE)"
  fi

  case "${ACCESS_MODE}" in
    "") if [[ -n "${SUBFLOW_DOMAIN}" ]]; then ACCESS_MODE="domain"; elif is_interactive; then prompt_access_mode; else ACCESS_MODE="ip"; fi ;;
    ip | 1 | direct) ACCESS_MODE="ip" ;;
    domain | 2 | caddy | https) ACCESS_MODE="domain" ;;
    *) die "Invalid SUBFLOW_ACCESS_MODE: ${ACCESS_MODE}. Use 'ip' or 'domain'." ;;
  esac

  if [[ "${ACCESS_MODE}" == "ip" ]]; then
    info "Access mode: direct IP on port ${SUBFLOW_PORT}."
    return 0
  fi

  if [[ -n "${SUBFLOW_DOMAIN}" ]]; then
    DOMAIN="$(normalize_domain "${SUBFLOW_DOMAIN}")"
  else
    DOMAIN="$(conf_get DOMAIN)"
  fi
  if [[ -z "${DOMAIN}" ]] && is_interactive; then
    prompt_domain
  fi
  valid_domain "${DOMAIN}" || die "Domain mode requires a valid domain; set SUBFLOW_DOMAIN or run interactively."
  [[ -n "${SUBFLOW_ACME_EMAIL}" ]] || SUBFLOW_ACME_EMAIL="$(conf_get ACME_EMAIL)"
  info "Access mode: domain ${DOMAIN} with automatic HTTPS (Caddy)."
}

caddy_restart() {
  if have_systemd; then systemctl restart "${CADDY_SERVICE}"; else rc-service "${CADDY_SERVICE}" restart; fi
}

caddy_enable() {
  if have_systemd; then systemctl enable "${CADDY_SERVICE}" > /dev/null 2>&1 || true; else rc-update add "${CADDY_SERVICE}" default > /dev/null 2>&1 || true; fi
}

install_caddy() {
  if command -v caddy > /dev/null 2>&1; then
    ok "Caddy is already installed."
    return 0
  fi
  info "Installing Caddy ..."
  if command -v apt-get > /dev/null 2>&1; then
    DEBIAN_FRONTEND=noninteractive apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq caddy
  elif command -v dnf > /dev/null 2>&1; then
    dnf install -y caddy
  elif command -v yum > /dev/null 2>&1; then
    yum install -y caddy
  elif command -v pacman > /dev/null 2>&1; then
    pacman -S --noconfirm caddy
  elif command -v zypper > /dev/null 2>&1; then
    zypper -n install caddy
  elif command -v apk > /dev/null 2>&1; then
    apk add --no-cache caddy
  else
    die "Could not install Caddy automatically. Install it, then re-run the installer."
  fi
  command -v caddy > /dev/null 2>&1 || die "Caddy installation did not succeed."
}

configure_caddy() {
  install -d -m 0755 "$(dirname "${CADDYFILE}")"
  if [[ -f "${CADDYFILE}" ]] && ! grep -q "Managed by the SubFlow installer" "${CADDYFILE}"; then
    cp -a "${CADDYFILE}" "${CADDYFILE}.bak" || true
    warn "Backed up the previous Caddyfile to ${CADDYFILE}.bak."
  fi
  umask 022
  if [[ -n "${SUBFLOW_ACME_EMAIL}" ]]; then
    cat > "${CADDYFILE}" << EOF
# Managed by the SubFlow installer. Manual edits may be overwritten on update.
{
	email ${SUBFLOW_ACME_EMAIL}
}

${DOMAIN} {
	encode gzip
	reverse_proxy 127.0.0.1:${SUBFLOW_PORT}
}
EOF
  else
    cat > "${CADDYFILE}" << EOF
# Managed by the SubFlow installer. Manual edits may be overwritten on update.
${DOMAIN} {
	encode gzip
	reverse_proxy 127.0.0.1:${SUBFLOW_PORT}
}
EOF
  fi
  ok "Wrote ${CADDYFILE} for ${DOMAIN}."
  if command -v caddy > /dev/null 2>&1; then
    caddy validate --config "${CADDYFILE}" --adapter caddyfile > /dev/null 2>&1 \
      || warn "Caddy reported issues while validating ${CADDYFILE}."
  fi
  caddy_enable
  caddy_restart
  ok "Caddy is running as a reverse proxy for ${DOMAIN}."
}

setup_domain_access() {
  install_caddy
  configure_caddy
  warn "Make sure ports 80 and 443 are open in your firewall and that ${DOMAIN} points to this server."
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
  if [[ "${ACCESS_MODE}" == "domain" ]]; then
    echo "  Web interface : https://${DOMAIN}/"
    echo "  Reverse proxy : Caddy (${CADDYFILE})"
  else
    echo "  Web interface : http://<server-ip>:${SUBFLOW_PORT}/"
  fi
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
  resolve_access_mode
  info "Installing SubFlow (${tag}) for ${target} ..."
  download_binary "${tag}" "${target}"
  write_env_file
  save_install_conf
  install_ctl
  install_service
  if [[ "${ACCESS_MODE}" == "domain" ]]; then
    setup_domain_access
  fi
  print_summary "${tag}" "${target}"
}

cmd_update() {
  need_root
  local target tag
  target="$(detect_target)"
  tag="$(resolve_latest_tag)"
  [[ -n "${tag}" ]] || die "Failed to resolve the latest release. GitHub API may be rate limited; retry later."
  resolve_access_mode
  info "Updating SubFlow to ${tag} for ${target} ..."
  download_binary "${tag}" "${target}"
  write_env_file
  save_install_conf
  install_ctl
  if [[ -f "${SYSTEMD_UNIT}" ]] || [[ -f "${OPENRC_UNIT}" ]]; then
    SERVICE_INSTALLED=true
    service_restart
  else
    install_service
  fi
  if [[ "${ACCESS_MODE}" == "domain" ]]; then
    setup_domain_access
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
  if [[ -f "${CADDYFILE}" ]] && grep -q "Managed by the SubFlow installer" "${CADDYFILE}"; then
    if [[ -f "${CADDYFILE}.bak" ]]; then
      mv -f "${CADDYFILE}.bak" "${CADDYFILE}"
      info "Restored the previous Caddyfile from ${CADDYFILE}.bak."
    else
      rm -f "${CADDYFILE}"
    fi
    if command -v caddy > /dev/null 2>&1; then
      caddy_restart > /dev/null 2>&1 || true
    fi
  fi
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
                       SUBFLOW_ACCESS_MODE (ip|domain), SUBFLOW_DOMAIN, SUBFLOW_ACME_EMAIL

When run with a terminal, the installer asks whether to expose SubFlow by IP
address or by a domain name with automatic HTTPS (via Caddy). Set the overrides
above to install unattended.
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
