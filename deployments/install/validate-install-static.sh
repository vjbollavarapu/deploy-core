#!/usr/bin/env bash
# Platform-independent static checks for the DeployCore Agent installer (I9).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
INSTALL="${ROOT}/deployments/install"
SCRIPT="${INSTALL}/install-agent.sh"
UNIT="${INSTALL}/deploycore-agent.service"
FAIL=0

pass() { echo "PASS: $*"; }
fail() { echo "FAIL: $*"; FAIL=1; }

echo "== I9 installer static validation =="

if bash -n "${SCRIPT}"; then
    pass "bash -n install-agent.sh"
else
    fail "bash -n install-agent.sh"
fi

# Must not use curl|bash install pattern (ignore comments)
if grep -vE '^[[:space:]]*#' "${SCRIPT}" | grep -E 'curl.*\|.*(ba)?sh|wget.*\|.*(ba)?sh' >/dev/null; then
    fail "curl|bash pattern detected"
else
    pass "no curl|bash install pattern"
fi

# Must not prune foreign Docker resources
if grep -vE '^[[:space:]]*#' "${SCRIPT}" | grep -E 'docker[[:space:]]+system[[:space:]]+prune|docker[[:space:]]+container[[:space:]]+prune' >/dev/null; then
    fail "dangerous docker prune found"
else
    pass "no docker system prune"
fi

# Must not configure removed redirect-https@file middleware as a dependency
if grep -vE '^[[:space:]]*#' "${SCRIPT}" "${UNIT}" | grep -F 'redirect-https@file' >/dev/null; then
    fail "stale redirect-https@file reference"
else
    pass "no redirect-https@file dependency"
fi

# systemd must not leak tokens on ExecStart
if grep -E 'ExecStart=.*TOKEN|ExecStart=.*token|ExecStart=.*--token' "${UNIT}"; then
    fail "token appears on ExecStart"
else
    pass "ExecStart has no token flags"
fi

# systemd must use EnvironmentFile + bare binary (Agent is env-configured)
if grep -q 'EnvironmentFile=.*/etc/deploycore-agent/agent.env' "${UNIT}" \
    && grep -q 'EnvironmentFile=.*/etc/deploycore-agent/registration.env' "${UNIT}" \
    && grep -qE '^ExecStart=/usr/local/bin/deploycore-agent$' "${UNIT}"; then
    pass "systemd EnvironmentFile + ExecStart aligned with Agent"
else
    fail "systemd unit not aligned with Agent env config"
fi

if grep -q 'User=deploycore' "${UNIT}" && grep -q 'Group=deploycore' "${UNIT}" \
    && grep -q 'SupplementaryGroups=docker' "${UNIT}" \
    && grep -q 'KillSignal=SIGTERM' "${UNIT}"; then
    pass "systemd User/Group/docker/SIGTERM"
else
    fail "systemd User/Group/docker/SIGTERM incomplete"
fi

# Installer must gate OS/arch and require checksum on remote path
for needle in 'ID' 'ubuntu' '22.04' '24.04' 'Unsupported system architecture' 'sha256sum' 'Checksum mismatch' 'deploycore-proxy' 'deploycore-traefik'; do
    if grep -qF "${needle}" "${SCRIPT}"; then
        :
    else
        fail "missing expected installer content: ${needle}"
    fi
done
pass "OS/arch/checksum/proxy markers present"

# Credential path must match Agent default naming (credentials.json)
if grep -q 'credentials.json' "${SCRIPT}"; then
    pass "credential path uses credentials.json"
else
    fail "credential path missing credentials.json"
fi

# Idempotency: must not wipe data dirs
if grep -E 'rm[[:space:]]+-rf[[:space:]]+/var/lib/deploycore-agent' "${SCRIPT}"; then
    fail "installer wipes data directory"
else
    pass "no data-directory wipe"
fi

if [ "${FAIL}" -ne 0 ]; then
    echo "== I9 static validation FAILED =="
    exit 1
fi
echo "== I9 static validation PASSED =="
