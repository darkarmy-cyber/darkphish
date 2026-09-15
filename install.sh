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
SERVICE_DROPIN_ETC="/etc/systemd/system/${SERVICE_NAME}.d"
SERVICE_DROPIN_RUN="/run/systemd/system/${SERVICE_NAME}.d"
GO_REQUIRED="1.27.1"
GO_SHA256_AMD64="63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445"
GO_SHA256_ARM64="3450b45a3f9ee8568792736a5c5e70a1f2e9b36c35a8f74958c03e51d7d92bec"
SOURCE_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
BUILD_ROOT=""
SOURCE_SNAPSHOT=""
SOURCE_SHA=""
SOURCE_VERSION=""
SOURCE_BUILT_AT=""
SOURCE_IS_RELEASE=0
GO_BIN=""
GO_ARCH=""
GO_EXPECTED_SHA256=""
CC_BIN=""
PKG_FAMILY=""
INSTALL_COMMITTED=0
APP_PATHS_CREATED=0
APP_GROUP_CREATED=0
APP_USER_CREATED=0
SERVICE_FILE_CREATED=0

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
    cat <<'USAGE_EOF'
Darkphish Linux installer

Usage:
  sudo ./install.sh
  ./install.sh --help

The installer performs a fresh production installation only. It refuses to
replace an existing Darkphish installation. Supported hosts are Linux amd64 or
arm64 systems using systemd 245 or newer and an apt, dnf, or yum package family.

Default layout:
  Application: /opt/darkphish
  Secrets/TLS: /etc/darkphish
  Bootstrap:   /var/lib/darkphish/bootstrap
  Service:     darkphish.service
USAGE_EOF
}

path_entry_exists() {
    [[ -e "$1" || -L "$1" ]]
}

git_source() {
    env -i PATH="${PATH}" HOME="/root" LC_ALL=C \
        git --no-replace-objects \
        -c "safe.directory=${SOURCE_DIR}" \
        -c core.fsmonitor=false \
        -C "${SOURCE_DIR}" "$@"
}

safe_tar() {
    env -i PATH="${PATH}" LC_ALL=C tar "$@"
}

openssl_safe() {
    env -i PATH="${PATH}" LC_ALL=C openssl "$@"
}

rollback_install() {
    set +e
    warn "installation did not complete; removing installer-created Darkphish artifacts"

    if [[ ${SERVICE_FILE_CREATED} -eq 1 ]]; then
        systemctl disable --now "${SERVICE_NAME}" >/dev/null 2>&1 || true
        rm -f -- "${SERVICE_FILE}"
        systemctl daemon-reload >/dev/null 2>&1 || true
        systemctl reset-failed "${SERVICE_NAME}" >/dev/null 2>&1 || true
    fi

    if [[ ${APP_PATHS_CREATED} -eq 1 ]]; then
        rm -rf -- "${INSTALL_DIR}" "${CONFIG_DIR}" "${STATE_DIR}"
    fi

    if [[ ${APP_USER_CREATED} -eq 1 ]]; then
        userdel "${APP_USER}" >/dev/null 2>&1 || true
    fi
    if [[ ${APP_GROUP_CREATED} -eq 1 ]]; then
        groupdel "${APP_GROUP}" >/dev/null 2>&1 || true
    fi
}

cleanup() {
    local status=$?
    trap - EXIT
    set +e
    if [[ ${status} -ne 0 && ${INSTALL_COMMITTED} -ne 1 ]]; then
        rollback_install
    fi
    if [[ -n "${BUILD_ROOT}" && -d "${BUILD_ROOT}" ]]; then
        rm -rf -- "${BUILD_ROOT}"
    fi
    exit "${status}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

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

prepare_build_root() {
    local base probe candidate
    for base in /var/tmp /tmp /root; do
        [[ -d "${base}" && -w "${base}" ]] || continue
        candidate="$(mktemp -d "${base%/}/darkphish-install.XXXXXX" 2>/dev/null || true)"
        [[ -n "${candidate}" ]] || continue
        chmod 0700 "${candidate}"
        probe="${candidate}/exec-probe"
        printf '#!/bin/sh\nexit 0\n' > "${probe}"
        chmod 0700 "${probe}"
        if env -i PATH="${PATH}" "${probe}" >/dev/null 2>&1; then
            rm -f -- "${probe}"
            BUILD_ROOT="${candidate}"
            return
        fi
        rm -rf -- "${candidate}"
    done
    die "no executable private temporary filesystem is available for the verified build; /var/tmp, /tmp, or /root must allow root execution"
}

verify_source_tree() {
    [[ -f "${SOURCE_DIR}/go.mod" && -f "${SOURCE_DIR}/go.sum" && -f "${SOURCE_DIR}/VERSION" && -f "${SOURCE_DIR}/config.json" ]] || die "run install.sh from the Darkphish repository root"
    [[ -d "${SOURCE_DIR}/db" && -d "${SOURCE_DIR}/templates" && -d "${SOURCE_DIR}/static/js/dist" && -d "${SOURCE_DIR}/static/css/dist" ]] || die "required runtime assets are missing from the repository"
    command -v git >/dev/null 2>&1 || die "git is required to verify the source tree"
    command -v tar >/dev/null 2>&1 || die "tar is required to prepare the verified source snapshot before host mutation"

    local top_level status_output
    if ! top_level="$(git_source rev-parse --show-toplevel 2>/dev/null)"; then
        die "the installer requires a valid Git checkout"
    fi
    [[ "${top_level}" == "${SOURCE_DIR}" ]] || die "install.sh must be run from the root of its Git checkout"

    if ! status_output="$(git_source status --porcelain=v1 --untracked-files=all 2>/dev/null)"; then
        die "Git working-tree cleanliness could not be verified"
    fi
    [[ -z "${status_output}" ]] || die "the Git working tree is not clean; commit, stash, or remove local changes before installing"

    if [[ -n "$(find "${SOURCE_DIR}/db" "${SOURCE_DIR}/templates" "${SOURCE_DIR}/static/images" "${SOURCE_DIR}/static/font" "${SOURCE_DIR}/static/db" "${SOURCE_DIR}/static/endpoint" "${SOURCE_DIR}/static/js/dist" "${SOURCE_DIR}/static/js/src" "${SOURCE_DIR}/static/css/dist" -type l -print -quit 2>/dev/null)" ]]; then
        die "runtime payload contains a symbolic link; refusing installation"
    fi
}

prepare_source_snapshot() {
    SOURCE_SHA="$(git_source rev-parse HEAD)" || die "cannot resolve source commit"
    [[ "${SOURCE_SHA}" =~ ^[0-9a-f]{40}$ ]] || die "source commit is malformed"

    SOURCE_VERSION="$(git_source show "${SOURCE_SHA}:VERSION" | tr -d '[:space:]')" || die "cannot read VERSION from source commit"
    [[ "${SOURCE_VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "VERSION does not contain a valid SemVer release"
    SOURCE_BUILT_AT="$(git_source show -s --format=%cI "${SOURCE_SHA}")" || die "cannot read source commit timestamp"

    if git_source tag --points-at "${SOURCE_SHA}" | grep -Fxq "v${SOURCE_VERSION}"; then
        SOURCE_IS_RELEASE=1
    else
        warn "HEAD is not the exact v${SOURCE_VERSION} tag; this will be a development build and verified in-product updates will remain unavailable"
    fi

    SOURCE_SNAPSHOT="${BUILD_ROOT}/source"
    mkdir -p -- "${SOURCE_SNAPSHOT}"
    if ! git_source archive --format=tar "${SOURCE_SHA}" | safe_tar -xf - -C "${SOURCE_SNAPSHOT}"; then
        die "failed to create immutable source snapshot from Git"
    fi

    [[ -f "${SOURCE_SNAPSHOT}/go.mod" && -d "${SOURCE_SNAPSHOT}/db" && -d "${SOURCE_SNAPSHOT}/templates" ]] || die "Git source snapshot is incomplete"
    if [[ -n "$(find "${SOURCE_SNAPSHOT}/db" "${SOURCE_SNAPSHOT}/templates" "${SOURCE_SNAPSHOT}/static/images" "${SOURCE_SNAPSHOT}/static/font" "${SOURCE_SNAPSHOT}/static/db" "${SOURCE_SNAPSHOT}/static/endpoint" "${SOURCE_SNAPSHOT}/static/js/dist" "${SOURCE_SNAPSHOT}/static/js/src" "${SOURCE_SNAPSHOT}/static/css/dist" -type l -print -quit 2>/dev/null)" ]]; then
        die "tracked runtime payload contains a symbolic link; refusing installation"
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
        x86_64|amd64)
            GO_ARCH="amd64"
            GO_EXPECTED_SHA256="${GO_SHA256_AMD64}"
            ;;
        aarch64|arm64)
            GO_ARCH="arm64"
            GO_EXPECTED_SHA256="${GO_SHA256_ARM64}"
            ;;
        *) die "unsupported CPU architecture: $(uname -m); supported: amd64, arm64" ;;
    esac

    command -v systemctl >/dev/null 2>&1 || die "systemd is required"
    command -v getent >/dev/null 2>&1 || die "getent is required for account preflight checks"
    [[ -d /run/systemd/system ]] || die "systemd is not running on this host"

    local manager_version systemd_version
    if ! manager_version="$(systemctl show --property=Version --value 2>/dev/null)"; then
        die "cannot connect to the running systemd manager"
    fi
    [[ "${manager_version}" =~ ^([0-9]+) ]] || die "cannot determine running systemd manager version"
    systemd_version="${BASH_REMATCH[1]}"
    (( systemd_version >= 245 )) || die "systemd 245 or newer is required for the configured hardening directives; found systemd ${systemd_version}"
}

refuse_existing_install() {
    local managed_path
    for managed_path in \
        "${INSTALL_DIR}" \
        "${CONFIG_DIR}" \
        "${STATE_DIR}" \
        "${SERVICE_FILE}" \
        "${SERVICE_DROPIN_ETC}" \
        "${SERVICE_DROPIN_RUN}" \
        "/run/systemd/system/${SERVICE_NAME}" \
        "/usr/lib/systemd/system/${SERVICE_NAME}" \
        "/lib/systemd/system/${SERVICE_NAME}"; do
        if path_entry_exists "${managed_path}"; then
            die "existing Darkphish service/runtime path found: ${managed_path}; refusing to overwrite it"
        fi
    done

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

    local command_name
    for command_name in curl git gcc openssl tar sha256sum stat; do
        command -v "${command_name}" >/dev/null 2>&1 || die "required command is unavailable after dependency installation: ${command_name}"
    done
    CC_BIN="$(command -v gcc)"
}

curl_https() {
    local url="$1"
    local output="$2"
    curl --disable --fail --location --silent --show-error \
        --proto '=https' --tlsv1.2 --retry 3 --retry-delay 2 \
        --output "${output}" "${url}"
}

ensure_go() {
    local installed=""
    if command -v go >/dev/null 2>&1; then
        installed="$(env -i PATH="${PATH}" HOME="/root" GOENV=off GOTOOLCHAIN=local go version 2>/dev/null | awk '{print $3}' || true)"
    fi
    if [[ "${installed}" == "go${GO_REQUIRED}" ]]; then
        GO_BIN="$(command -v go)"
        log "Using installed Go ${GO_REQUIRED}"
        return
    fi

    log "Bootstrapping pinned Go ${GO_REQUIRED} toolchain for linux/${GO_ARCH}"
    local archive="${BUILD_ROOT}/go${GO_REQUIRED}.linux-${GO_ARCH}.tar.gz"
    local url="https://go.dev/dl/go${GO_REQUIRED}.linux-${GO_ARCH}.tar.gz"
    curl_https "${url}" "${archive}"

    [[ "${GO_EXPECTED_SHA256}" =~ ^[0-9a-f]{64}$ ]] || die "internal Go checksum pin is malformed"
    printf '%s  %s\n' "${GO_EXPECTED_SHA256}" "${archive}" | sha256sum --check --status - || die "Go toolchain checksum verification failed"

    mkdir -p -- "${BUILD_ROOT}/toolchain"
    safe_tar -xzf "${archive}" -C "${BUILD_ROOT}/toolchain"
    GO_BIN="${BUILD_ROOT}/toolchain/go/bin/go"
    [[ -x "${GO_BIN}" ]] || die "verified Go toolchain did not extract correctly"
    [[ "$(env -i PATH="${PATH}" HOME="/root" GOENV=off GOTOOLCHAIN=local "${GO_BIN}" version | awk '{print $3}')" == "go${GO_REQUIRED}" ]] || die "unexpected Go toolchain version after extraction"
}

build_darkphish() {
    local release_ldflag="" ldflags
    if [[ ${SOURCE_IS_RELEASE} -eq 1 ]]; then
        release_ldflag=" -X main.releaseVersion=${SOURCE_VERSION}"
    fi
    ldflags="-s -w -X main.commitSHA=${SOURCE_SHA} -X main.builtAt=${SOURCE_BUILT_AT}${release_ldflag}"

    log "Building Darkphish ${SOURCE_VERSION} from ${SOURCE_SHA}"
    mkdir -p -- "${BUILD_ROOT}/home" "${BUILD_ROOT}/gomodcache" "${BUILD_ROOT}/gocache"

    (
        cd -- "${SOURCE_SNAPSHOT}"
        env -i \
            PATH="${PATH}" \
            HOME="${BUILD_ROOT}/home" \
            GOCACHE="${BUILD_ROOT}/gocache" \
            GOMODCACHE="${BUILD_ROOT}/gomodcache" \
            GOENV=off \
            GOWORK=off \
            GOTOOLCHAIN=local \
            GOFLAGS='-mod=readonly' \
            GOPROXY='https://proxy.golang.org,direct' \
            GOSUMDB='sum.golang.org' \
            GOPRIVATE='' \
            GONOPROXY='' \
            GONOSUMDB='' \
            CGO_ENABLED=1 \
            GOOS=linux \
            GOARCH="${GO_ARCH}" \
            CC="${CC_BIN}" \
            "${GO_BIN}" build -trimpath -ldflags "${ldflags}" -o "${BUILD_ROOT}/darkphish" ./
    )

    [[ -x "${BUILD_ROOT}/darkphish" ]] || die "Darkphish build did not produce an executable"
    env -i PATH="${PATH}" "${BUILD_ROOT}/darkphish" version >/dev/null || die "built Darkphish executable failed its version smoke check"
}

create_service_account() {
    log "Creating dedicated system account"
    groupadd --system "${APP_GROUP}"
    APP_GROUP_CREATED=1

    local nologin_shell
    nologin_shell="$(command -v nologin || true)"
    [[ -n "${nologin_shell}" ]] || nologin_shell="/usr/sbin/nologin"
    useradd --system --gid "${APP_GROUP}" --home-dir "${INSTALL_DIR}" --no-create-home --shell "${nologin_shell}" "${APP_USER}"
    APP_USER_CREATED=1
}

generate_key_file() {
    local path="$1"
    local value
    value="$(openssl_safe rand -base64 32 | tr -d '\r\n')"
    [[ -n "${value}" ]] || die "failed to generate cryptographic key material"
    umask 0077
    printf 'base64:%s\n' "${value}" > "${path}"
}

install_payload() {
    log "Installing application payload"
    APP_PATHS_CREATED=1
    install -d -m 0750 -o "${APP_USER}" -g "${APP_GROUP}" "${INSTALL_DIR}"
    install -d -m 0750 -o root -g "${APP_GROUP}" "${CONFIG_DIR}"
    install -d -m 0750 -o root -g "${APP_GROUP}" "${CONFIG_DIR}/tls"
    install -d -m 0750 -o "${APP_USER}" -g "${APP_GROUP}" "${STATE_DIR}"
    install -d -m 0700 -o "${APP_USER}" -g "${APP_GROUP}" "${BOOTSTRAP_DIR}"

    install -m 0750 -o "${APP_USER}" -g "${APP_GROUP}" "${BUILD_ROOT}/darkphish" "${INSTALL_DIR}/darkphish"
    local file_name
    for file_name in VERSION LICENSE NOTICE.md README.md CHANGELOG.md; do
        install -m 0640 -o "${APP_USER}" -g "${APP_GROUP}" "${SOURCE_SNAPSHOT}/${file_name}" "${INSTALL_DIR}/${file_name}"
    done

    cp -a -- "${SOURCE_SNAPSHOT}/db" "${INSTALL_DIR}/db"
    cp -a -- "${SOURCE_SNAPSHOT}/templates" "${INSTALL_DIR}/templates"
    mkdir -p -- "${INSTALL_DIR}/static/js" "${INSTALL_DIR}/static/css"
    cp -a -- "${SOURCE_SNAPSHOT}/static/images" "${SOURCE_SNAPSHOT}/static/font" "${SOURCE_SNAPSHOT}/static/db" "${SOURCE_SNAPSHOT}/static/endpoint" "${INSTALL_DIR}/static/"
    cp -a -- "${SOURCE_SNAPSHOT}/static/js/dist" "${SOURCE_SNAPSHOT}/static/js/src" "${INSTALL_DIR}/static/js/"
    cp -a -- "${SOURCE_SNAPSHOT}/static/css/dist" "${INSTALL_DIR}/static/css/"

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

    local openssl_config="${BUILD_ROOT}/openssl-admin.cnf"
    cat > "${openssl_config}" <<'OPENSSL_EOF'
[req]
distinguished_name = dn
prompt = no
x509_extensions = v3_req

[dn]
CN = localhost

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
OPENSSL_EOF

    openssl_safe req -x509 -newkey rsa:3072 -sha256 -days 825 -nodes \
        -keyout "${CONFIG_DIR}/tls/admin.key" \
        -out "${CONFIG_DIR}/tls/admin.crt" \
        -config "${openssl_config}" \
        -extensions v3_req >/dev/null 2>&1

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
    cat > "${INSTALL_DIR}/config.json" <<EOF_CONFIG
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
EOF_CONFIG
    chown "${APP_USER}:${APP_GROUP}" "${INSTALL_DIR}/config.json"
    chmod 0640 "${INSTALL_DIR}/config.json"
}

create_systemd_unit() {
    log "Creating hardened systemd service"
    SERVICE_FILE_CREATED=1
    cat > "${SERVICE_FILE}" <<EOF_SERVICE
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
EOF_SERVICE
    chown root:root "${SERVICE_FILE}"
    chmod 0644 "${SERVICE_FILE}"

    if command -v systemd-analyze >/dev/null 2>&1; then
        systemd-analyze verify "${SERVICE_FILE}" >/dev/null || die "systemd unit validation failed"
    fi
}

verify_loaded_systemd_unit() {
    systemctl daemon-reload

    local fragment dropins effective_user effective_group
    fragment="$(systemctl show "${SERVICE_NAME}" -p FragmentPath --value)" || die "cannot inspect loaded systemd unit"
    [[ "${fragment}" == "${SERVICE_FILE}" ]] || die "systemd loaded Darkphish from an unexpected unit path: ${fragment:-unknown}"

    dropins="$(systemctl show "${SERVICE_NAME}" -p DropInPaths --value)" || die "cannot inspect systemd drop-ins"
    [[ -z "${dropins}" ]] || die "unexpected systemd drop-ins affect ${SERVICE_NAME}: ${dropins}"

    effective_user="$(systemctl show "${SERVICE_NAME}" -p User --value)" || die "cannot inspect systemd service user"
    effective_group="$(systemctl show "${SERVICE_NAME}" -p Group --value)" || die "cannot inspect systemd service group"
    [[ "${effective_user}" == "${APP_USER}" && "${effective_group}" == "${APP_GROUP}" ]] || die "systemd service identity differs from the dedicated Darkphish account"
}

start_service() {
    log "Enabling and starting Darkphish"
    verify_loaded_systemd_unit
    systemctl enable --now "${SERVICE_NAME}" >/dev/null

    local attempt password_file current_pid stable_pid="" ready_streak=0
    password_file="${BOOTSTRAP_DIR}/darkphish_initial_admin_password"
    for attempt in $(seq 1 60); do
        current_pid="$(systemctl show "${SERVICE_NAME}" -p MainPID --value 2>/dev/null || true)"
        if [[ "${current_pid}" =~ ^[1-9][0-9]*$ ]] && \
            systemctl is-active --quiet "${SERVICE_NAME}" && \
            curl --disable --fail --silent --show-error --max-time 2 \
                --cacert "${CONFIG_DIR}/tls/admin.crt" \
                "https://127.0.0.1:3333/readyz" >/dev/null 2>&1 && \
            curl --disable --silent --show-error --max-time 2 \
                --output /dev/null "http://127.0.0.1:80/" >/dev/null 2>&1 && \
            [[ -f "${password_file}" && ! -L "${password_file}" && -s "${password_file}" ]]; then
            if [[ "${current_pid}" == "${stable_pid}" ]]; then
                ready_streak=$((ready_streak + 1))
            else
                stable_pid="${current_pid}"
                ready_streak=1
            fi
            if (( ready_streak >= 5 )); then
                return
            fi
        else
            stable_pid=""
            ready_streak=0
        fi
        sleep 1
    done

    systemctl --no-pager --full status "${SERVICE_NAME}" || true
    journalctl --no-pager -u "${SERVICE_NAME}" -n 50 || true
    die "Darkphish did not become application-ready on both admin and simulation listeners; inspect: journalctl -u ${SERVICE_NAME}"
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
    if [[ ! -x /usr/bin/gh ]]; then
        warn "optional one-click native updates require a separately installed /usr/bin/gh >= 2.100.0; see docs/UPDATES.md"
    fi
    if [[ ${SOURCE_IS_RELEASE} -ne 1 ]]; then
        warn "this installation was built from an untagged source commit; verified in-product updates are intentionally unavailable"
    fi
}

main() {
    require_root
    detect_platform
    refuse_existing_install
    prepare_build_root

    verify_source_tree
    prepare_source_snapshot
    install_system_packages
    ensure_go
    build_darkphish
    create_service_account
    install_payload
    create_security_material
    create_production_config
    create_systemd_unit
    start_service

    INSTALL_COMMITTED=1
    print_completion
}

main
