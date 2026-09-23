#!/usr/bin/env bash
# DeployCore Server Agent Installer for Ubuntu LTS
# Documentation: https://github.com/deploycore/deploy-core
#
# Agent runtime configuration is environment-based (AGENT_*).
# This installer writes /etc/deploycore-agent/agent.env for systemd EnvironmentFile=.

set -euo pipefail

# Handle --help and -h before privilege escalation
for arg in "$@"; do
    if [ "$arg" = "-h" ] || [ "$arg" = "--help" ]; then
        echo "Usage: $0 [options]"
        echo "Options:"
        echo "  --install-docker         Install Docker Engine using official Ubuntu repository"
        echo "  --server-url URL         DeployCore Control Plane URL (required on first install)"
        echo "  --token TOKEN            One-time server registration token (first registration only)"
        echo "  --server-id UUID         Designated Server ID"
        echo "  --version VER            Agent release version tag (default: latest)"
        echo "  --binary PATH            Local binary to install instead of downloading"
        echo "  --checksum FILE|URL      Path or URL to SHA256SUMS (required for remote download)"
        echo "  --release-url BASE       Release download base URL"
        echo "  --acme-email EMAIL       Email for Traefik Let's Encrypt ACME (optional)"
        echo "  --skip-traefik           Skip Traefik edge proxy prerequisite setup"
        exit 0
    fi
done

# 1. Root Escalation Check
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    echo ">> Elevating privileges using sudo..."
    exec sudo -E bash "$0" "$@"
fi

# Configuration Defaults
INSTALL_DOCKER=false
SERVER_URL=""
REG_TOKEN=""
SERVER_ID=""
VERSION="${DEPLOYCORE_VERSION:-latest}"
BINARY_SOURCE=""
CHECKSUM_FILE=""
RELEASE_BASE="${DEPLOYCORE_RELEASE_URL:-https://github.com/deploycore/deploy-core/releases/download}"
ACME_EMAIL="${DEPLOYCORE_ACME_EMAIL:-}"
SKIP_TRAEFIK=false

USER_NAME="deploycore"
GROUP_NAME="deploycore"
BIN_DEST="/usr/local/bin/deploycore-agent"
CONFIG_DIR="/etc/deploycore-agent"
ENV_FILE="${CONFIG_DIR}/agent.env"
REG_ENV_FILE="${CONFIG_DIR}/registration.env"
DATA_DIR="/var/lib/deploycore-agent"
CREDENTIAL_PATH="${DATA_DIR}/credentials.json"
LOG_DIR="/var/log/deploycore-agent"
RUN_DIR="/run/deploycore-agent"
TRAEFIK_DATA_DIR="${DATA_DIR}/traefik"
PROXY_NETWORK="deploycore-proxy"
TRAEFIK_CONTAINER="deploycore-traefik"
ARTIFACT_PREFIX="deploycore-agent-linux"

# Parse Command Line Arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --install-docker)
            INSTALL_DOCKER=true
            shift
            ;;
        --server-url)
            SERVER_URL="$2"
            shift 2
            ;;
        --token)
            REG_TOKEN="$2"
            shift 2
            ;;
        --server-id)
            SERVER_ID="$2"
            shift 2
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        --binary)
            BINARY_SOURCE="$2"
            shift 2
            ;;
        --checksum)
            CHECKSUM_FILE="$2"
            shift 2
            ;;
        --release-url)
            RELEASE_BASE="$2"
            shift 2
            ;;
        --acme-email)
            ACME_EMAIL="$2"
            shift 2
            ;;
        --skip-traefik)
            SKIP_TRAEFIK=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

echo "=================================================="
echo "    DeployCore Server Agent Installer             "
echo "=================================================="

# 2. Supported OS contract (Ubuntu LTS)
if [ ! -f /etc/os-release ]; then
    echo "ERROR: /etc/os-release not found. This installer supports Ubuntu LTS only." >&2
    exit 1
fi
# shellcheck source=/dev/null
. /etc/os-release
if [ "${ID:-}" != "ubuntu" ]; then
    echo "ERROR: Unsupported OS '${ID:-unknown}'. Only Ubuntu LTS is supported." >&2
    exit 1
fi
case "${VERSION_ID:-}" in
    22.04|24.04)
        echo ">> Ubuntu ${VERSION_ID} (${VERSION_CODENAME:-}) accepted."
        ;;
    *)
        echo "ERROR: Unsupported Ubuntu version '${VERSION_ID:-unknown}'. Supported: 22.04, 24.04." >&2
        exit 1
        ;;
esac

# 3. Architecture Validation
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)
        TARGET_ARCH="amd64"
        ;;
    aarch64|arm64)
        TARGET_ARCH="arm64"
        ;;
    *)
        echo "ERROR: Unsupported system architecture: ${ARCH}. Only amd64 and arm64 are supported." >&2
        exit 1
        ;;
esac
echo ">> Architecture detected: ${ARCH} (target: ${TARGET_ARCH})"
ARTIFACT_NAME="${ARTIFACT_PREFIX}-${TARGET_ARCH}"

require_cmd() {
    if ! command -v "$1" &>/dev/null; then
        echo "ERROR: required command not found: $1" >&2
        exit 1
    fi
}

# 4. Optional Docker Engine Installation (official Ubuntu repo)
if [ "${INSTALL_DOCKER}" = true ]; then
    if command -v docker &>/dev/null && docker info &>/dev/null; then
        echo ">> Docker already installed and healthy; skipping reinstall."
    else
        echo ">> Installing Docker Engine from official repository..."
        require_cmd apt-get
        apt-get update -qq
        apt-get install -y -qq ca-certificates curl gnupg
        install -m 0755 -d /etc/apt/keyrings
        curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg --yes
        chmod a+r /etc/apt/keyrings/docker.gpg
        echo \
          "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
          ${VERSION_CODENAME} stable" | \
          tee /etc/apt/sources.list.d/docker.list > /dev/null
        apt-get update -qq
        apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
        systemctl enable --now docker
    fi
fi

# 5. Validate Docker Installation
if ! command -v docker &>/dev/null; then
    echo "ERROR: Docker is not installed. Re-run with --install-docker or install Docker manually." >&2
    exit 1
fi
if ! docker info &>/dev/null; then
    echo "ERROR: Docker daemon is not running or inaccessible." >&2
    exit 1
fi
echo ">> Docker daemon check passed."

# 6. Create System User and Group (idempotent)
if ! getent group "${GROUP_NAME}" >/dev/null; then
    echo ">> Creating system group: ${GROUP_NAME}"
    groupadd --system "${GROUP_NAME}"
fi
if ! getent passwd "${USER_NAME}" >/dev/null; then
    echo ">> Creating system user: ${USER_NAME}"
    useradd --system --gid "${GROUP_NAME}" --no-create-home \
        --shell /usr/sbin/nologin \
        --comment "DeployCore Agent Service User" "${USER_NAME}"
else
    echo ">> System user ${USER_NAME} already exists."
fi

if getent group docker >/dev/null; then
    usermod -aG docker "${USER_NAME}"
    echo ">> Ensured ${USER_NAME} is a member of the docker group."
else
    echo "WARN: docker group not found; Agent may be unable to access the Docker socket." >&2
fi

# 7. Create Directories with Restrictive Permissions (idempotent; never wipe data)
echo ">> Setting up directory hierarchy..."
mkdir -p "${CONFIG_DIR}"
mkdir -p "${DATA_DIR}"/{workspaces,backups,updates}
mkdir -p "${TRAEFIK_DATA_DIR}"
mkdir -p "${LOG_DIR}"
mkdir -p "${RUN_DIR}"

chown -R "${USER_NAME}:${GROUP_NAME}" "${CONFIG_DIR}"
chmod 0750 "${CONFIG_DIR}"

chown -R "${USER_NAME}:${GROUP_NAME}" "${DATA_DIR}"
chmod 0700 "${DATA_DIR}"
chmod 0700 "${DATA_DIR}/workspaces" "${DATA_DIR}/backups" "${DATA_DIR}/updates" "${TRAEFIK_DATA_DIR}"

# ACME storage must be writable by Traefik container (often root inside container)
# Keep host dir owner deploycore; file created with 0600 on first Traefik start.
touch "${TRAEFIK_DATA_DIR}/acme.json"
chown "${USER_NAME}:${GROUP_NAME}" "${TRAEFIK_DATA_DIR}/acme.json"
chmod 0600 "${TRAEFIK_DATA_DIR}/acme.json"

chown -R "${USER_NAME}:${GROUP_NAME}" "${LOG_DIR}"
chmod 0750 "${LOG_DIR}"

chown -R "${USER_NAME}:${GROUP_NAME}" "${RUN_DIR}"
chmod 0755 "${RUN_DIR}"

# 8. Install Agent Binary (checksum required for remote; optional but enforced when provided for local)
verify_sha256() {
    local file_path="$1"
    local sums_source="$2"
    local expected=""
    local actual=""
    local sums_file=""
    local tmp_sums=""

    if [ -z "${sums_source}" ]; then
        echo "ERROR: checksum source required for verification." >&2
        return 1
    fi

    if [[ "${sums_source}" =~ ^https?:// ]]; then
        tmp_sums="$(mktemp)"
        if ! curl -fsSL "${sums_source}" -o "${tmp_sums}"; then
            rm -f "${tmp_sums}"
            echo "ERROR: failed to download checksum file." >&2
            return 1
        fi
        sums_file="${tmp_sums}"
    else
        if [ ! -f "${sums_source}" ]; then
            echo "ERROR: checksum file not found: ${sums_source}" >&2
            return 1
        fi
        sums_file="${sums_source}"
    fi

    expected="$(grep -E "(^|\\s)$(basename "${file_path}")\$" "${sums_file}" | awk '{print $1}' | head -n1 || true)"
    if [ -z "${expected}" ]; then
        # Also allow matching by artifact name regardless of local basename
        expected="$(grep -F "${ARTIFACT_NAME}" "${sums_file}" | awk '{print $1}' | head -n1 || true)"
    fi
    if [ -z "${expected}" ]; then
        [ -n "${tmp_sums:-}" ] && rm -f "${tmp_sums}"
        echo "ERROR: no SHA-256 entry found for $(basename "${file_path}") in checksum metadata." >&2
        return 1
    fi

    actual="$(sha256sum "${file_path}" | awk '{print $1}')"
    [ -n "${tmp_sums:-}" ] && rm -f "${tmp_sums}"

    if [ "${expected}" != "${actual}" ]; then
        echo "ERROR: Checksum mismatch! Expected ${expected}, got ${actual}" >&2
        return 1
    fi
    echo ">> Checksum verified."
}

install_binary_from_path() {
    local src="$1"
    if [ ! -f "${src}" ]; then
        echo "ERROR: binary not found: ${src}" >&2
        exit 1
    fi
    if [ -n "${CHECKSUM_FILE}" ]; then
        echo ">> Verifying checksum..."
        verify_sha256 "${src}" "${CHECKSUM_FILE}"
    else
        echo "WARN: installing local binary without checksum verification (--checksum not provided)." >&2
    fi
    install -m 0755 -o root -g root "${src}" "${BIN_DEST}"
    echo ">> Installed binary to ${BIN_DEST}"
}

download_and_install_release() {
    require_cmd curl
    require_cmd sha256sum

    local tag="${VERSION}"
    local staging
    staging="$(mktemp -d)"
    # shellcheck disable=SC2064
    cleanup_staging() { rm -rf "${staging}"; }
    trap cleanup_staging EXIT

    local bin_url sums_url
    if [ "${tag}" = "latest" ]; then
        cleanup_staging
        trap - EXIT
        echo "ERROR: remote install with --version latest requires an explicit version tag, or use --binary." >&2
        echo "       Set --version vX.Y.Z and --checksum <SHA256SUMS path or URL>." >&2
        exit 1
    fi

    bin_url="${RELEASE_BASE}/${tag}/${ARTIFACT_NAME}"
    if [ -n "${CHECKSUM_FILE}" ]; then
        sums_url="${CHECKSUM_FILE}"
    else
        sums_url="${RELEASE_BASE}/${tag}/SHA256SUMS"
    fi

    echo ">> Downloading Agent artifact for ${tag} (${TARGET_ARCH})..."
    if ! curl -fsSL "${bin_url}" -o "${staging}/${ARTIFACT_NAME}"; then
        cleanup_staging
        trap - EXIT
        echo "ERROR: failed to download Agent binary from release URL." >&2
        exit 1
    fi

    echo ">> Verifying SHA-256 against trusted checksum metadata..."
    if ! verify_sha256 "${staging}/${ARTIFACT_NAME}" "${sums_url}"; then
        cleanup_staging
        trap - EXIT
        exit 1
    fi
    install -m 0755 -o root -g root "${staging}/${ARTIFACT_NAME}" "${BIN_DEST}"
    cleanup_staging
    trap - EXIT
    echo ">> Installed binary to ${BIN_DEST}"
}

if [ -n "${BINARY_SOURCE}" ]; then
    echo ">> Installing binary from local path..."
    install_binary_from_path "${BINARY_SOURCE}"
elif [ ! -f "${BIN_DEST}" ] || [ "${VERSION}" != "latest" ]; then
    # Remote path: require explicit version + checksum verification (no curl|bash)
    download_and_install_release
else
    echo ">> Existing binary at ${BIN_DEST}; leaving in place (re-run with --binary or --version to replace)."
fi

if [ ! -f "${BIN_DEST}" ]; then
    echo "ERROR: Agent binary missing at ${BIN_DEST}." >&2
    exit 1
fi

# 9. Generate Environment Configuration (Agent loads AGENT_* from environment)
# Idempotent: do not overwrite an existing env file (preserves identity wiring).
# One-time registration token lives ONLY in registration.env (removed after success).
CREDENTIALS_EXIST=false
if [ -f "${CREDENTIAL_PATH}" ]; then
    CREDENTIALS_EXIST=true
fi

if [ ! -f "${ENV_FILE}" ]; then
    if [ -z "${SERVER_URL}" ]; then
        echo "ERROR: --server-url is required on first install to write Agent configuration." >&2
        exit 1
    fi
    echo ">> Writing Agent environment file at ${ENV_FILE}..."
    {
        echo "# DeployCore Agent environment — managed by install-agent.sh"
        echo "# Do not put secrets on the process command line."
        echo "AGENT_CONTROL_PLANE_URL=${SERVER_URL}"
        echo "AGENT_DATA_DIR=${DATA_DIR}"
        echo "AGENT_CREDENTIAL_PATH=${CREDENTIAL_PATH}"
        echo "AGENT_LOG_LEVEL=info"
        echo "AGENT_HEARTBEAT_INTERVAL=30s"
        if [ -n "${SERVER_ID}" ]; then
            echo "AGENT_SERVER_ID=${SERVER_ID}"
        fi
    } > "${ENV_FILE}"
    chown "root:${GROUP_NAME}" "${ENV_FILE}"
    chmod 0640 "${ENV_FILE}"
else
    echo ">> Existing ${ENV_FILE} preserved (idempotent; not overwritten)."
    # Scrub any legacy token lines from older installer revisions (do not print values).
    if grep -q '^AGENT_REGISTRATION_TOKEN=' "${ENV_FILE}" 2>/dev/null; then
        echo ">> Removing registration token from durable agent.env (moved to registration.env lifecycle)..."
        grep -v '^AGENT_REGISTRATION_TOKEN=' "${ENV_FILE}" > "${ENV_FILE}.tmp"
        mv "${ENV_FILE}.tmp" "${ENV_FILE}"
        chown "root:${GROUP_NAME}" "${ENV_FILE}"
        chmod 0640 "${ENV_FILE}"
    fi
fi

# registration.env: first-boot only; Agent deletes this file after successful registration.
if [ "${CREDENTIALS_EXIST}" = true ]; then
    if [ -f "${REG_ENV_FILE}" ]; then
        echo ">> Credentials already present; removing spent ${REG_ENV_FILE}..."
        rm -f "${REG_ENV_FILE}"
    fi
elif [ -n "${REG_TOKEN}" ]; then
    echo ">> Writing one-time registration bootstrap file (token value redacted)..."
    {
        echo "# One-time registration token — deleted by Agent after successful registration"
        echo "AGENT_REGISTRATION_TOKEN=${REG_TOKEN}"
    } > "${REG_ENV_FILE}"
    chown "root:${GROUP_NAME}" "${REG_ENV_FILE}"
    chmod 0640 "${REG_ENV_FILE}"
elif [ ! -f "${REG_ENV_FILE}" ]; then
    echo "WARN: no --token and no existing credentials; Agent will start unregistered until a token is provided." >&2
fi

# Remove stale YAML config that Agent does not read (legacy A32 draft)
if [ -f "${CONFIG_DIR}/agent.yaml" ]; then
    echo ">> Removing unused legacy ${CONFIG_DIR}/agent.yaml (Agent uses AGENT_* env, not YAML)."
    rm -f "${CONFIG_DIR}/agent.yaml"
fi

# 10. Proxy network + Traefik edge prerequisite (Docker provider; no redirect-https@file)
ensure_proxy_network() {
    if docker network inspect "${PROXY_NETWORK}" &>/dev/null; then
        echo ">> Docker network ${PROXY_NETWORK} already exists."
        return 0
    fi
    echo ">> Creating Docker network ${PROXY_NETWORK}..."
    docker network create \
        --label "deploycore.managed=true" \
        --label "deploycore.protected=true" \
        --label "deploycore.network_type=proxy" \
        "${PROXY_NETWORK}" >/dev/null
}

ensure_traefik() {
    if [ "${SKIP_TRAEFIK}" = true ]; then
        echo ">> Skipping Traefik setup (--skip-traefik)."
        return 0
    fi

    ensure_proxy_network

    if docker inspect "${TRAEFIK_CONTAINER}" &>/dev/null; then
        echo ">> Traefik container ${TRAEFIK_CONTAINER} already present; leaving untouched."
        return 0
    fi

    echo ">> Starting Traefik edge proxy (${TRAEFIK_CONTAINER}) on ${PROXY_NETWORK}..."
    # Traefik uses Docker provider labels only (I5 redirectscheme middleware).
    # No file-provider redirect-https@file dependency.
    local -a run_args=(
        run -d
        --name "${TRAEFIK_CONTAINER}"
        --restart unless-stopped
        --network "${PROXY_NETWORK}"
        -p 80:80
        -p 443:443
        -v /var/run/docker.sock:/var/run/docker.sock:ro
        -v "${TRAEFIK_DATA_DIR}:/data"
        --label "deploycore.managed=true"
        --label "deploycore.protected=true"
        --label "deploycore.service_type=edge"
        "traefik:v3.3"
        --providers.docker=true
        --providers.docker.exposedbydefault=false
        --providers.docker.network="${PROXY_NETWORK}"
        --entrypoints.web.address=:80
        --entrypoints.websecure.address=:443
    )
    if [ -n "${ACME_EMAIL}" ]; then
        run_args+=(
            "--certificatesresolvers.letsencrypt.acme.email=${ACME_EMAIL}"
            "--certificatesresolvers.letsencrypt.acme.storage=/data/acme.json"
            "--certificatesresolvers.letsencrypt.acme.httpchallenge=true"
            "--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=web"
        )
    else
        echo "ERROR: --acme-email is required for Traefik Let's Encrypt (certresolver 'letsencrypt')." >&2
        echo "       Pass --acme-email, or use --skip-traefik for hosts without public HTTPS." >&2
        exit 1
    fi

    docker "${run_args[@]}" >/dev/null
    echo ">> Traefik started. ACME data: ${TRAEFIK_DATA_DIR}/acme.json"
}

ensure_traefik

# 11. Install Systemd Service Unit
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ ! -f "${SCRIPT_DIR}/deploycore-agent.service" ]; then
    echo "ERROR: deploycore-agent.service not found next to installer at ${SCRIPT_DIR}" >&2
    exit 1
fi
echo ">> Installing systemd service unit..."
cp "${SCRIPT_DIR}/deploycore-agent.service" /etc/systemd/system/deploycore-agent.service
chmod 0644 /etc/systemd/system/deploycore-agent.service
systemctl daemon-reload
echo ">> Systemd daemon reloaded."

# 12. Summary Report (no secrets)
echo ""
echo "=================================================="
echo "    DeployCore Agent Installation Complete!       "
echo "=================================================="
echo "Config env: ${ENV_FILE}"
echo "Data:       ${DATA_DIR}"
echo "Credential: ${CREDENTIAL_PATH}"
echo "Binary:     ${BIN_DEST}"
echo "Service:    deploycore-agent.service"
echo "Proxy net:  ${PROXY_NETWORK}"
if [ "${SKIP_TRAEFIK}" = false ]; then
    echo "Traefik:    ${TRAEFIK_CONTAINER}"
fi
echo ""
echo "To start the service:"
echo "  sudo systemctl enable --now deploycore-agent"
echo "Registration token (if any) is stored only in ${REG_ENV_FILE}"
echo "  and is deleted by the Agent after successful registration."
echo "Restarts use ${CREDENTIAL_PATH}; a new registration token is not required."
echo "To inspect status:"
echo "  sudo systemctl status deploycore-agent"
echo "To view logs:"
echo "  sudo journalctl -u deploycore-agent -f"
echo "=================================================="
