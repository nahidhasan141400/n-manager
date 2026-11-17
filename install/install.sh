#!/usr/bin/env bash
# Install or upgrade N Manager. Existing config and database files are kept.
set -euo pipefail

BINARY=""
URL="${NMANAGER_URL:-}"
CHECKSUM=""
PREFIX="/usr"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY="$2"; shift 2 ;;
    --url) URL="$2"; shift 2 ;;
    --checksum) CHECKSUM="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this installer as root." >&2
  exit 1
fi

if [[ -r /etc/os-release ]]; then
  # shellcheck disable=SC1091
  . /etc/os-release
else
  echo "Unable to detect the Linux distribution." >&2
  exit 1
fi

id="${ID:-}"
like="${ID_LIKE:-}"
blob="${id} ${like}"
if [[ "$blob" == *debian* || "$blob" == *ubuntu* || "$blob" == *linuxmint* || "$blob" == *pop* ]]; then
  FAMILY="apt"
elif [[ "$blob" == *fedora* || "$blob" == *rhel* || "$blob" == *centos* || "$blob" == *rocky* || "$blob" == *alma* ]]; then
  FAMILY="dnf"
else
  echo "Unsupported distribution: ${PRETTY_NAME:-$id}" >&2
  exit 1
fi

arch="$(uname -m)"
case "$arch" in
  x86_64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
if [[ -n "$BINARY" ]]; then
  install -m 0755 "$BINARY" "$tmp"
  if [[ -n "$CHECKSUM" ]]; then
    echo "${CHECKSUM}  ${tmp}" | sha256sum -c -
  fi
elif [[ -n "$URL" ]]; then
  if [[ -z "$CHECKSUM" ]]; then
    echo "A sha256 checksum is required when downloading the binary." >&2
    exit 1
  fi
  curl -fsSL "$URL" -o "$tmp"
  echo "${CHECKSUM}  ${tmp}" | sha256sum -c -
  chmod 0755 "$tmp"
else
  echo "Provide --binary or --url. Detected ${FAMILY} ${GOARCH}." >&2
  exit 1
fi

install -d -m 0755 /usr/lib/n-manager /usr/bin
if ! id nmanager >/dev/null 2>&1; then
  useradd --system --home-dir /var/lib/n-manager --shell /usr/sbin/nologin nmanager
fi
if ! id nmanager-deploy >/dev/null 2>&1; then
  useradd --system --create-home --home-dir /var/lib/n-manager/deploy --shell /usr/sbin/nologin nmanager-deploy
fi
install -d -o nmanager -g nmanager -m 0750 /etc/n-manager /var/lib/n-manager /var/log/n-manager
install -d -o nmanager-deploy -g nmanager-deploy -m 0750 /var/lib/n-manager/deploy
install -m 0755 "$tmp" /usr/bin/n-manager
install -m 0755 "$(dirname "$0")/../deploy/nvm-run.sh" /usr/lib/n-manager/nvm-run.sh
install -m 0440 "$(dirname "$0")/../deploy/n-manager.sudoers" /etc/sudoers.d/n-manager
install -m 0644 "$(dirname "$0")/../deploy/n-manager.service" /etc/systemd/system/n-manager.service

if [[ ! -f /etc/n-manager/config.yaml ]]; then
  if [[ -n "${NMANAGER_ADMIN_PASSWORD:-}" ]]; then
    pass="$NMANAGER_ADMIN_PASSWORD"
  else
    read -r -s -p "Admin password: " pass
    echo
    read -r -s -p "Confirm password: " pass2
    echo
    if [[ "$pass" != "$pass2" ]]; then
      echo "Passwords do not match." >&2
      exit 1
    fi
  fi
  hash="$(printf '%s' "$pass" | /usr/bin/n-manager hash-password)"
  secret="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
  cat > /etc/n-manager/config.yaml <<EOF
server:
  host: "127.0.0.1"
  port: 8080
  tls_cert: ""
  tls_key: ""
security:
  admin_password_hash: "${hash}"
  session_secret: "${secret}"
  session_ttl_minutes: 720
  cookie_secure: "auto"
  dry_run: false
database:
  driver: "sqlite"
  path: "/var/lib/n-manager/nmanager.db"
monitoring:
  refresh_interval: 5
privileged:
  command: "/usr/bin/n-manager"
EOF
  chown nmanager:nmanager /etc/n-manager/config.yaml
  chmod 0640 /etc/n-manager/config.yaml
  echo "Created /etc/n-manager/config.yaml"
else
  echo "Keeping existing configuration."
fi

if [[ -f /var/lib/n-manager/nmanager.db ]]; then
  echo "Keeping existing database."
fi

systemctl daemon-reload
systemctl enable --now n-manager
systemctl restart n-manager
echo "N Manager is running at http://127.0.0.1:8080"
