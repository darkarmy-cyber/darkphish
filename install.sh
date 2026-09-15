#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

APP_NAME="darkphish"
APP_USER="darkphish"
APP_GROUP="darkphish"
SERVICE_NAME="darkphish.service"
INSTALL_DIR="/opt/darkphish"
CONFIG_DIR="/etc/darkphish"
STATE_DIR="/var/lib/darkphish"
BOOTSTRAP_DIR="${STATE_DIR}/bootstrap"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}"
GO_REQUIRED="1.27.1"
SOURCE_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
BUILD_ROOT=""
GO_BIN=""
GO_ARCH=""
PKG_FAMILY=""

log() {
    printf '[darkphish] %s\n' "$*"
}

warn() {
    printf '[darkphish] WARNING: %s\n' "$*" >&2
}

die() {
    printf '[darkphish] ERROR: %s\n' "$*" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Darkphish Linux installer

Usage:
  sudo ./install.sh
  ./install.sh --help

The installer performs a fresh production installation only. It refuses to
replace an existing Darkphish installation. Supported hosts are Linux amd64 or
arm64 systems using systemd and an apt, dnf, or yum package family.

Default layout:
  Application: /opt/darkphish
  Secrets/TLS: /etc/darkphish
  Bootstrap:   /var/lib/darkphish/bootstrap
  Service:     darkphish.service
EOF
}

cleanup() {
    if [[ -n "${BUILD_ROOT}" && -d "${BUILD_ROOT}" ]]; then
        rm -rf -- "${BUILD_ROOT}"
    fi
}
trap cleanup EXIT

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    usage
    exit 0
fi
if [[ $# -ne 0 ]]; then
    usage >&2
    exit 2
fi

require_root() {
    [[ ${EUID} -eq 0 ]] || die "root privileges are required; run: sudo ./install.sh"
}

verify_source_tree() {
    cd -- "${SOURCE_DIR}"
    [[ -f go.mod && -f go.sum && -f VERSION && -f config.json ]] || die "run install.sh from the Darkphish repository root"
    [[ -d db && -d templates && -d static/js/dist && -d static/css/dist ]] || die "required runtime assets are missing from the repository"
    command -v git >/dev/null 2>&1 || die "git is required to verify the source tree"
    git rev-parse --is-inside-work-tree >/dev/null 2>&1 || die "the installer requires a Git checkout"
    [[ "$(git rev-parse --show-toplevel)" == "${SOURCE_DIR}" ]] || die "install.sh must be run from the root of its Git checkout"
    if [[ -n "$(git status --porcelain --untracked-files=all)" ]]; then
        die "the Git working tree is not clean; commit, stash, or remove local changes before installing"
    fi
    if [[ -n "$(find db templates static/images static/font static/db static/endpoint static/js/dist static/js/src static/css/dist -type l -print -quit 2>/dev/null)" ]]; then
        die "runtime payload contains a symbolic link; refusing installation"
    fi
}

detect_platform() {
    [[ "$(uname -s)" == "Linux" ]] || die "this installer currently supports Linux only"
    [[ -r /etc/os-release ]] || die "cannot identify Linux distribution (/etc/os-release missing)"
    # shellcheck disable=SC1091
    . /etc/os-release

    local identity="${ID:-} ${ID_LIKE:-}"
    case "${identity}" in
        *debian*|*ubuntu*) PKG_FAMILY="apt" ;;
        *rhel*|*fedora*|*centos*|*rocky*|*almalinux*|*ol*)
            if command -v dnf >/dev/null 2>&1; then
                PKG_FAMILY="dnf"
            elif command -v yum >/dev/null 2>&1; then
                PKG_FAMILY="yum"
            else
                die "RHEL-family host detected but neither dnf nor yum is available"
            fi
            ;;
        *) die "unsupported Linux distribution: ${PRETTY_NAME:-${ID:-unknown}}" ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64) GO_ARCH="amd64" ;;
        aarch64|arm64) GO_ARCH="arm64" ;;
        *) die "unsupported CPU architecture: $(uname -m); supported: amd64, arm64" ;;
    esac

    command -v systemctl >/dev/null 2>&1 || die "systemd is required"
    [[ -d /run/systemd/system ]] || die "systemd is not running on this host"
}

refuse_existing_install() {
    if [[ -e "${INSTALL_DIR}" || -e "${CONFIG_DIR}" || -e "${STATE_DIR}" || -e "${SERVICE_FILE}" ]]; then
        die "an existing Darkphish path was found; this installer is intentionally fresh-install only"
    fi
    if systemctl cat "${SERVICE_NAME}" >/dev/null 2>&1; then
        die "${SERVICE_NAME} already exists; refusing to overwrite it"
    fi
    if id "${APP_USER}" >/dev/null 2>&1 || getent group "${APP_GROUP}" >/dev/null 2>&1; then
        die "user or group '${APP_USER}' already exists; refusing an ambiguous installation"
    fi
}

install_system_packages() {
    log "Installing required build and runtime packages (${PKG_FAMILY})"
    case "${PKG_FAMILY}" in
        apt)
            export DEBIAN_FRONTEND=noninteractive
            apt-get update
            apt-get install -y --no-install-recommends \
                ca-certificates curl git build-essential openssl tar gzip xz-utils passwd
            ;;
        dnf)
            dnf -y install \
                ca-certificates curl git gcc gcc-c++ make glibc-devel openssl tar gzip xz shadow-utils
            ;;
        yum)
            yum -y install \
                ca-certificates curl git gcc gcc-c++ make glibc-devel openssl tar gzip xz shadow-utils
            ;;
        *) die "internal error: unsupported package family" ;;
    esac

    for command_name in curl git gcc openssl tar sha256sum; do
        command -v "${command_name}" >/dev/null 2>&1 || die "required command is unavailable after dependency installation: ${command_name}"
    done
}

curl_https() {
    local url="$1"
    local output="$2"
    curl --fail --location --silent --show-error \
        --proto '=https' --tlsv1.2 --retry 3 --retry-delay 2 \
        --output "${output}" "${url}"
}

ensure_go() {
    local installed=""
    if command -v go >/dev/null 2>&1; then
        installed="$(go version 2>/dev/null | awk '{print $3}' || true)"
    fi
    if [[ "${installed}" == "go${GO_REQUIRED}" ]]; then
        GO_BIN="$(command -v go)"
        log "Using installed Go ${GO_REQUIRED}"
        return
    fi

    log "Bootstrapping verified Go ${GO_REQUIRED} toolchain for linux/${GO_ARCH}"
    local archive="${BUILD_ROOT}/go${GO_REQUIRED}.linux-${GO_ARCH}.tar.gz"
    local checksum_file="${archive}.sha256"
    local url="https://go.dev/dl/go${GO_REQUIRED}.linux-${GO_ARCH}.tar.gz"
    curl_https "${url}" "${archive}"
    curl_https "${url}.sha256" "${checksum_file}"

    local expected
    expected="$(tr -d '[:space:]' < "${checksum_file}")"
    [[ "${expected}" =~ ^[0-9a-fA-F]{64}$ ]] || die "Go checksum response is malformed"
    printf '%s  %s\n' "${expected}" "${archive}" | sha256sum --check --status - || die "Go toolchain checksum verification failed"

    mkdir -p -- "${BUILD_ROOT}/toolchain"
    tar -xzf "${archive}" -C "${BUILD_ROOT}/toolchain"
    GO_BIN="${BUILD_ROOT}/toolchain/go/bin/go"
    [[ -x "${GO_BIN}" ]] || die "verified Go toolchain did not extract correctly"
    [[ "$(${GO_BIN} version | awk '{print $3}')" == "go${GO_REQUIRED}" ]] || die "unexpected Go toolchain version after extraction"
}

build_darkphish() {
    cd -- "${SOURCE_DIR}"
    local version commit_sha built_at release_ldflag ldflags
    version="$(tr -d '[:space:]' < VERSION)"
    [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "VERSION does not contain a valid SemVer release"
    commit_sha="$(git rev-parse HEAD)"
    built_at="$(git show -s --format=%cI HEAD)"
    release_ldflag=""

    if git tag --points-at "${commit_sha}" | grep -Fxq "v${version}"; then
        release_ldflag=" -X main.releaseVersion=${version}"
    else
        warn "HEAD is not the exact v${version} tag; the installed binary will correctly identify itself as a development build"
    fi

    ldflags="-s -w -X main.commitSHA=${commit_sha} -X main.builtAt=${built_at}${release_ldflag}"
    log "Building Darkphish ${version} from ${commit_sha}"
    GOTOOLCHAIN=local GOFLAGS='-mod=readonly' CGO_ENABLED=1 GOOS=linux GOARCH="${GO_ARCH}" \
        "${GO_BIN}" build -trimpath -ldflags "${ldflags}" -o "${BUILD_ROOT}/darkphish" ./
    [[ -x "${BUILD_ROOT}/darkphish" ]] || die "Darkphish build did not produce an executable"
    "${BUILD_ROOT}/darkphish" version >/dev/null || die "built Darkphish executable failed its version smoke check"
}

create_service_account() {
    log "Creating dedicated system account"
    groupadd --system "${APP_GROUP}"
    local nologin_shell
    nologin_shell="$(command -v nologin || true)"
    [[ -n "${nologin_shell}" ]] || nologin_shell="/usr/sbin/nologin"
    useradd --system --gid "${APP_GROUP}" --home-dir "${INSTALL_DIR}" --no-create-home --shell "${nologin_shell}" "${APP_USER}"
}

generate_key_file() {
    local path="$1"
    local value
    value="$(openssl rand -base64 32 | tr -d '\r\n')"
    [[ -n "${value}" ]] || die "failed to generate cryptographic key material"
    umask 0077
    printf 'base64:%s\n' "${value}" > "${path}"
}

install_payload() {
    log "Installing application payload"
    install -d -m 0750 -o "${APP_USER}" -g "${APP_GROUP}" "${INSTALL_DIR}"
    install -d -m 0750 -o root -g "${APP_GROUP}" "${CONFIG_DIR}"
    install -d -m 0750 -o root -g "${APP_GROUP}" "${CONFIG_DIR}/tls"
    install -d -m 0750 -o "${APP_USER}" -g "${APP_GROUP}" "${STATE_DIR}"
    install -d -m 0700 -o "${APP_USER}" -g "${APP_GROUP}" "${BOOTSTRAP_DIR}"

    install -m 0750 -o "${APP_USER}" -g "${APP_GROUP}" "${BUILD_ROOT}/darkphish" "${INSTALL_DIR}/darkphish"
    for file_name in VERSION LICENSE NOTICE.md README.md CHANGELOG.md; do
        install -m 0640 -o "${APP_USER}" -g "${APP_GROUP}" "${SOURCE_DIR}/${file_name}" "${INSTALL_DIR}/${file_name}"
    done

    cp -a -- "${SOURCE_DIR}/db" "${INSTALL_DIR}/db"
    cp -a -- "${SOURCE_DIR}/templates" "${INSTALL_DIR}/templates"
    mkdir -p -- "${INSTALL_DIR}/static/js" "${INSTALL_DIR}/static/css"
    cp -a -- "${SOURCE_DIR}/static/images" "${SOURCE_DIR}/static/font" "${SOURCE_DIR}/static/db" "${SOURCE_DIR}/static/endpoint" "${INSTALL_DIR}/static/"
    cp -a -- "${SOURCE_DIR}/static/js/dist" "${SOURCE_DIR}/static/js/src" "${INSTALL_DIR}/static/js/"
    cp -a -- "${SOURCE_DIR}/static/css/dist" "${INSTALL_DIR}/static/css/"

    chown -R "${APP_USER}:${APP_GROUP}" "${INSTALL_DIR}"
    find "${INSTALL_DIR}" -type d -exec chmod 0750 {} +
    find "${INSTALL_DIR}" -type f -exec chmod 0640 {} +
    chmod 0750 "${INSTALL_DIR}/darkphish"
}

create_security_material() {
    log "Generating production key material and administrative TLS certificate"
    generate_key_file "${CONFIG_DIR}/session-auth.key"
    generate_key_file "${CONFIG_DIR}/session-encryption.key"
    generate_key_file "${CONFIG_DIR}/secrets.key"
    generate_key_file "${CONFIG_DIR}/audit-signing.key"

    openssl req -x509 -newkey rsa:3072 -sha256 -days 825 -nodes \
        -keyout "${CONFIG_DIR}/tls/admin.key" \
        -out "${CONFIG_DIR}/tls/admin.crt" \
        -subj '/CN=localhost' \
        -addext 'subjectAltName=DNS:localhost,IP:127.0.0.1' >/dev/null 2>&1

    chown root:"${APP_GROUP}" \
        "${CONFIG_DIR}/session-auth.key" \
        "${CONFIG_DIR}/session-encryption.key" \
        "${CONFIG_DIR}/secrets.key" \
        "${CONFIG_DIR}/audit-signing.key" \
        "${CONFIG_DIR}/tls/admin.key" \
        "${CONFIG_DIR}/tls/admin.crt"
    chmod 0640 \
        "${CONFIG_DIR}/session-auth.key" \
        "${CONFIG_DIR}/session-encryption.key" \
        "${CONFIG_DIR}/secrets.key" \
        "${CONFIG_DIR}/audit-signing.key" \
        "${CONFIG_DIR}/tls/admin.key" \
        "${CONFIG_DIR}/tls/admin.crt"
}

create_production_config() {
    log "Writing production configuration"
    umask 0077
    cat > "${INSTALL_DIR}/config.json" <<EOF
{
  "admin_server": {
    "listen_url": "127.0.0.1:3333",
    "use_tls": true,
    "cert_path": "${CONFIG_DIR}/tls/admin.crt",
    "key_path": "${CONFIG_DIR}/tls/admin.key",
    "trusted_origins": ["https://localhost:3333", "https://127.0.0.1:3333"],
    "cors_allowed_origins": [],
    "allowed_internal_hosts": [],
    "max_request_body_bytes": 16777216
  },
  "phish_server": {
    "listen_url": "0.0.0.0:80",
    "use_tls": false,
    "cert_path": "",
    "key_path": ""
  },
  "db_name": "sqlite3",
  "db_path": "darkphish.db",
  "db_max_open_conns": 1,
  "db_max_idle_conns": 1,
  "db_connection_max_lifetime_minutes": 30,
  "postgresql": {
    "host": "127.0.0.1",
    "port": 5432,
    "database": "darkphish",
    "username": "darkphish",
    "password_file": "",
    "sslmode": "verify-full",
    "connect_timeout_seconds": 10
  },
  "migrations_prefix": "db/db_",
  "production_mode": true,
  "session": {
    "auth_key_file": "${CONFIG_DIR}/session-auth.key",
    "encryption_key_file": "${CONFIG_DIR}/session-encryption.key",
    "lifetime_hours": 24
  },
  "secrets": {
    "provider": "local",
    "active_key_id": "",
    "keys": {},
    "encryption_key_file": "${CONFIG_DIR}/secrets.key",
    "vault": {
      "address": "",
      "token_file": "",
      "namespace": "",
      "mount": "transit",
      "key_name": "darkphish-secrets",
      "ca_cert": ""
    }
  },
  "audit": {
    "retention_days": 365,
    "checkpoint_interval": 1000,
    "active_signing_key_id": "",
    "signing_keys": {},
    "signing_key_file": "${CONFIG_DIR}/audit-signing.key"
  },
  "privileged_access": {
    "window_minutes": 5
  },
  "personal_access_tokens": {
    "max_lifetime_days": 90
  },
  "bootstrap_directory": "${BOOTSTRAP_DIR}",
  "contact_address": "",
  "logging": {
    "filename": "",
    "level": "info"
  }
}
EOF
    chown "${APP_USER}:${APP_GROUP}" "${INSTALL_DIR}/config.json"
    chmod 0640 "${INSTALL_DIR}/config.json"
}

create_systemd_unit() {
    log "Creating hardened systemd service"
    cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=Darkphish authorized phishing simulation platform
Documentation=https://github.com/darkarmy-cyber/darkphish
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=${APP_USER}
Group=${APP_GROUP}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/darkphish --config ${INSTALL_DIR}/config.json
Restart=on-failure
RestartSec=5s
TimeoutStopSec=120s
KillMode=control-group
UMask=0077

NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
ProtectHostname=true
RestrictSUIDSGID=true
LockPersonality=true
RestrictRealtime=true
RestrictNamespaces=true
SystemCallArchitectures=native
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE
ReadWritePaths=${INSTALL_DIR} ${BOOTSTRAP_DIR}

[Install]
WantedBy=multi-user.target
EOF
    chown root:root "${SERVICE_FILE}"
    chmod 0644 "${SERVICE_FILE}"

    if command -v systemd-analyze >/dev/null 2>&1; then
        systemd-analyze verify "${SERVICE_FILE}" >/dev/null || die "systemd unit validation failed"
    fi
}

start_service() {
    log "Enabling and starting Darkphish"
    systemctl daemon-reload
    systemctl enable --now "${SERVICE_NAME}" >/dev/null

    local attempt
    for attempt in $(seq 1 30); do
        if systemctl is-active --quiet "${SERVICE_NAME}"; then
            return
        fi
        sleep 1
    done

    systemctl --no-pager --full status "${SERVICE_NAME}" || true
    die "Darkphish did not reach the active state; inspect: journalctl -u ${SERVICE_NAME}"
}

print_completion() {
    local password_file="${BOOTSTRAP_DIR}/darkphish_initial_admin_password"
    printf '\n'
    log "Installation completed successfully."
    printf '\n'
    printf '  Admin URL:       https://127.0.0.1:3333\n'
    printf '  Admin username:  admin\n'
    printf '  Service:         %s\n' "${SERVICE_NAME}"
    printf '  Application:     %s\n' "${INSTALL_DIR}"
    printf '  Configuration:   %s/config.json\n' "${INSTALL_DIR}"
    printf '\n'
    printf 'Initial administrator password (owner-only file):\n'
    printf '  sudo cat %s\n' "${password_file}"
    printf '\n'
    printf 'For remote administration, keep the admin listener on loopback and use an SSH tunnel:\n'
    printf '  ssh -L 3333:127.0.0.1:3333 <user>@<server>\n'
    printf 'Then open https://localhost:3333, accept the temporary self-signed certificate,\n'
    printf 'sign in as admin, and change the bootstrap password immediately.\n'
    printf '\n'
    printf 'The simulation listener is active on TCP/80. Before Internet exposure, configure\n'
    printf 'DNS/TLS/reverse-proxy policy and review docs/DEPLOYMENT.md.\n'
    printf '\n'
    printf 'Useful commands:\n'
    printf '  systemctl status darkphish\n'
    printf '  journalctl -u darkphish -f\n'
    printf '  systemctl restart darkphish\n'
    printf '\n'
    if [[ ! -f "${password_file}" ]]; then
        warn "the bootstrap password file is not present yet; verify service logs before attempting login"
    fi
    if [[ ! -x /usr/bin/gh ]]; then
        warn "optional one-click native updates require a separately installed /usr/bin/gh >= 2.100.0; see docs/UPDATES.md"
    fi
}

main() {
    require_root
    verify_source_tree
    detect_platform
    refuse_existing_install

    BUILD_ROOT="$(mktemp -d /var/tmp/darkphish-install.XXXXXX)"
    chmod 0700 "${BUILD_ROOT}"

    install_system_packages
    ensure_go
    build_darkphish
    create_service_account
    install_payload
    create_security_material
    create_production_config
    create_systemd_unit
    start_service
    print_completion
}

main
