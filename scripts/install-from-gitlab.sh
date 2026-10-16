#!/usr/bin/env bash
# install-from-gitlab.sh — установка/обновление WebKVM на целевой машине
# из предсобранных артефактов локального GitLab. Ничего не компилирует:
# скачивает webkvm-linux-amd64.tar.gz из generic packages / releases.
#
# Использование:
#   sudo ./install-from-gitlab.sh                    # установить/обновить последнюю версию
#   sudo VERSION=1.2.0 ./install-from-gitlab.sh      # конкретная версия (без "v")
#   sudo GITLAB_URL=http://gitlab.local:8081 ./install-from-gitlab.sh
#   sudo ./install-from-gitlab.sh --uninstall
#
# Env:
#   GITLAB_URL  — базовый URL GitLab   (default: http://localhost:8081)
#   PROJECT     — namespace/repo       (default: root/webkvm)
#   VERSION     — версия без "v"       (default: latest release)
#   TOKEN       — personal access token с read_api (для приватного проекта)

set -euo pipefail

GITLAB_URL="${GITLAB_URL:-http://localhost:8081}"
PROJECT="${PROJECT:-root/webkvm}"
TOKEN="${TOKEN:-}"
DO_UNINSTALL=0

for arg in "$@"; do
  case "$arg" in
    --uninstall) DO_UNINSTALL=1 ;;
    -h|--help)
      sed -n '2,15p' "$0"; exit 0 ;;
    *)
      echo "ERROR: unknown argument: $arg (see --help)" >&2; exit 2 ;;
  esac
done

log()  { echo "[webkvm-install] $*"; }
die()  { echo "[webkvm-install] ERROR: $*" >&2; exit 1; }
need_root() { [ "$(id -u)" -eq 0 ] || die "run as root (sudo)"; }

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar  >/dev/null 2>&1 || die "tar is required"

# --- GitLab API helpers -------------------------------------------------
PROJECT_ENC="${PROJECT//\//%2F}"
API_BASE="${GITLAB_URL}/api/v4/projects/${PROJECT_ENC}"

api_get() {
  # api_get <path> — GET JSON, с токеном если задан.
  if [ -n "$TOKEN" ]; then
    curl -fsSL --header "PRIVATE-TOKEN: ${TOKEN}" "$1"
  else
    curl -fsSL "$1"
  fi
}

api_download() {
  # api_download <url> <outfile>
  if [ -n "$TOKEN" ]; then
    curl -fsSL --header "PRIVATE-TOKEN: ${TOKEN}" -o "$2" "$1"
  else
    curl -fsSL -o "$2" "$1"
  fi
}

resolve_version() {
  # VERSION не задан — берём последний release через permalink.
  if [ -n "${VERSION:-}" ]; then return 0; fi
  log "resolving latest release..."
  # permalink отдаёт 302; -L идёт за ним, версия — в конечном URL тега.
  local last
  last="$(api_get "${API_BASE}/releases/permalink/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  [ -n "$last" ] || die "cannot resolve latest release (project has releases? private? set TOKEN)"
  VERSION="${last#v}"
  log "latest release: v${VERSION}"
}

asset_url() {
  # Артефакты лежат в generic packages (см. .gitlab-ci.yml job upload).
  echo "${API_BASE}/packages/generic/webkvm/${VERSION}/webkvm-linux-amd64.tar.gz"
}

# --- Uninstall -----------------------------------------------------------
if [ "$DO_UNINSTALL" -eq 1 ]; then
  need_root
  log "uninstalling webkvm..."
  systemctl disable --now webkvm 2>/dev/null || true
  rm -f /etc/systemd/system/webkvm.service /etc/logrotate.d/webkvm
  rm -f /usr/local/bin/webkvm /usr/local/bin/webkvm.previous /usr/local/bin/webkvm-cli
  systemctl daemon-reload
  log "uninstalled. Data in /opt/webkvm and logs in /var/log/webkvm were kept — remove manually if desired."
  exit 0
fi

# --- Install / update ----------------------------------------------------
need_root
resolve_version

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  PKG_ARCH="amd64" ;;
  aarch64|arm64) PKG_ARCH="arm64" ;;
  *) die "unsupported architecture: $ARCH (artifacts are built for amd64/arm64)" ;;
esac
# Артефакты CI собираются только под amd64; arm64 требует доп. job в .gitlab-ci.yml.
[ "$PKG_ARCH" = "amd64" ] || die "no prebuilt artifact for ${PKG_ARCH} yet — add a build job in .gitlab-ci.yml"

TMPDIR_INSTALL="$(mktemp -d /tmp/webkvm-install.XXXXXX)"
trap 'rm -rf "$TMPDIR_INSTALL"' EXIT

log "downloading webkvm v${VERSION} (${PKG_ARCH}) from ${GITLAB_URL}..."
api_download "$(asset_url)" "${TMPDIR_INSTALL}/webkvm.tar.gz" \
  || die "download failed (version v${VERSION} exists? project private? set TOKEN)"
mkdir -p "${TMPDIR_INSTALL}/unpack"
tar -xzf "${TMPDIR_INSTALL}/webkvm.tar.gz" -C "${TMPDIR_INSTALL}/unpack"
[ -f "${TMPDIR_INSTALL}/unpack/webkvm" ] || die "tarball does not contain 'webkvm' binary"

# Сохраняем текущий бинарь для отката (как make rollback).
BIN=/usr/local/bin/webkvm
if [ -f "$BIN" ] && [ ! -f "${BIN}.previous" ]; then
  cp -a "$BIN" "${BIN}.previous"
  log "saved previous binary to ${BIN}.previous"
fi

log "installing..."
install -m 0755 "${TMPDIR_INSTALL}/unpack/webkvm" "$BIN"
[ -f "${TMPDIR_INSTALL}/unpack/webkvm-cli" ] && install -m 0755 "${TMPDIR_INSTALL}/unpack/webkvm-cli" /usr/local/bin/webkvm-cli
install -m 0644 "${TMPDIR_INSTALL}/unpack/webkvm.service" /etc/systemd/system/webkvm.service
if [ -f "${TMPDIR_INSTALL}/unpack/webkvm.logrotate" ]; then
  install -m 0644 "${TMPDIR_INSTALL}/unpack/webkvm.logrotate" /etc/logrotate.d/webkvm
fi

# Конфиг-файл создаём только если его нет — не затираем настройки.
# BIND_ADDR=0.0.0.0: без фронт-прокси (Caddy) UI должен быть доступен
# по сети; unit по умолчанию держит бинарь на loopback.
if [ ! -f /etc/default/webkvm ]; then
  cat > /etc/default/webkvm <<'EOF'
# WebKVM service configuration (sourced by systemd EnvironmentFile).
PORT=8091
# Bind on all interfaces — remove if a reverse proxy terminates traffic.
BIND_ADDR=0.0.0.0
EOF
  log "created /etc/default/webkvm (PORT=8091, BIND_ADDR=0.0.0.0)"
fi

mkdir -p /opt/webkvm /var/log/webkvm
systemctl daemon-reload
systemctl enable webkvm >/dev/null 2>&1 || true
systemctl restart webkvm

health_port="${PORT:-8091}"
health_ok=0
log "waiting for health check (up to 20s)..."
for _ in $(seq 1 20); do
  if curl -fsS -m 2 "http://127.0.0.1:${health_port}/api/health" >/dev/null 2>&1; then
    health_ok=1
    break
  fi
  sleep 1
done

if [ "$health_ok" -eq 1 ]; then
  log "health check passed — webkvm v${VERSION} installed and running"
  log "rollback if needed: sudo mv ${BIN}.previous ${BIN} && sudo systemctl restart webkvm"
  exit 0
fi

# --- Health check failed: roll back --------------------------------------
log "ERROR: health check did not pass within 20s — rolling back" >&2
systemctl stop webkvm 2>/dev/null || true
if [ -f "${BIN}.previous" ]; then
  mv "${BIN}.previous" "$BIN"
  systemctl restart webkvm
  log "rolled back to previous binary; check 'systemctl status webkvm'"
else
  log "no previous binary to roll back to — service left stopped" >&2
fi
exit 1
