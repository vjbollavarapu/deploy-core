# OCI Live Validation Checklist

Use this checklist **after I12** (or during the first Ubuntu OCI validation window) to prove DeployCore end-to-end on real Docker/Ubuntu.

Companion runbook: [production-runbook.md](./production-runbook.md).

**Rules**

- Do not simulate Docker success.
- Do not mark items PASS without evidence on the live host/UI/API.
- Redact secrets in any attached notes (`<REDACTED>`).

---

## A. Ubuntu / installation

- [ ] Host is Ubuntu **22.04** or **24.04**
- [ ] Architecture is **amd64** or **arm64**
- [ ] Installer rejects wrong OS/arch if tested
- [ ] `install-agent.sh` run with `--binary`, `--checksum`, `--install-docker`, `--server-url`, `--server-id`, `--token`, `--acme-email`
- [ ] Checksum mismatch aborts (spot-check with bad sum if safe)
- [ ] No `curl \| bash` install path used
- [ ] `deploycore` system user/group present; membership in `docker` group
- [ ] Paths exist with expected permissions (`/etc/deploycore-agent`, `/var/lib/deploycore-agent`, credentials parent 0700)

## B. Agent / Docker

- [ ] Docker Engine installed and `docker info` succeeds
- [ ] `systemctl enable --now deploycore-agent` succeeds
- [ ] Agent registers once; `credentials.json` created (0600)
- [ ] `registration.env` removed after successful registration
- [ ] Restart Agent **without** registration token; identity unchanged
- [ ] Heartbeat advances in Control Plane
- [ ] Command polling operational
- [ ] Server reaches **ONLINE** when Docker healthy (or **DEGRADED** only when Docker intentionally down)

## C. First deployment

- [ ] Project + environment + application created
- [ ] Application `target_server_id` = registered server
- [ ] Manual deployment created via UI/API
- [ ] Agent receives typed commands (not shell strings)
- [ ] Image pull/build boundary executes on real Docker
- [ ] Revision created; container created/started
- [ ] Deployment reaches expected success path **or** truthful structured failure
- [ ] Job `succeeded` not misread as deployment success when failure occurs (R4 awareness)

## D. Traefik / DNS / HTTPS

- [ ] Container `deploycore-traefik` running
- [ ] Network `deploycore-proxy` exists
- [ ] Domain attached in CP; DNS points to host
- [ ] HTTP reaches Traefik
- [ ] HTTPS works with Let’s Encrypt (`letsencrypt` resolver)
- [ ] HTTP→HTTPS redirect works via redirectscheme labels (no `redirect-https@file`)
- [ ] Candidate containers do not receive public routing before activation

## E. Zero-downtime / rollback

- [ ] New revision deploys as candidate
- [ ] Health gate blocks unhealthy activation
- [ ] Activation switches routing without requiring rebuild of prior image
- [ ] Previous revision retired/drained
- [ ] Rollback to READY/INACTIVE revision succeeds
- [ ] Active routing preserved across Agent restart (**LIVE**)

## F. Logs / metrics / events

- [ ] Runtime logs visible for application
- [ ] Build/pull logs visible and redacted for obvious secrets
- [ ] Agent cannot ingest logs for apps on another server (same org)
- [ ] Continuous metrics appear after Agent ingest
- [ ] Frontend `/metrics` shows CP data (demo mode off)
- [ ] Docker event stream reconnects after brief Docker blip

## G. Database

- [ ] Managed PostgreSQL provisions on Agent host
- [ ] Credential reveal audited / gated
- [ ] Volume persisted and protected
- [ ] Failed provision remains FAILED across Agent restart
- [ ] Soft-delete does not claim runtime stop/volume delete
- [ ] CP PostgreSQL never used as managed DB

## H. Backup / restore

- [ ] Backup created; status terminal success/failure truthful
- [ ] Checksum metadata present
- [ ] Restore requires confirm phrase **`RESTORE`**
- [ ] Restore validates target identity
- [ ] Interrupted restore does not mark success
- [ ] Local single-host limitation understood (R17)

## I. Failure / recovery

- [ ] Stop Agent → server becomes OFFLINE per TTL; no false success
- [ ] Restart Agent → same identity; polling/heartbeat resume
- [ ] Restart API (PG intact) → Agent reconnects; durable rows remain
- [ ] Docker stop → DEGRADED + structured command failures
- [ ] Expired/stuck commands terminalize via CP expiry
- [ ] Failed deployment not promoted by reconcile/restart

## J. Reboot / persistence

- [ ] Host reboot → Docker starts → Traefik returns → Agent systemd starts
- [ ] Credentials + journal survive
- [ ] Heartbeat/polling resume without new registration token
- [ ] Managed containers rediscovered / reconciled per design

## K. Foreign-resource safety

- [ ] Non-DeployCore containers/volumes/networks untouched by install/update
- [ ] No indiscriminate `docker system prune` performed by product paths
- [ ] Protected DB volumes survive soft-delete of CP database row
- [ ] Traefik/proxy labeled managed/protected as installed

---

## Sign-off

| Field | Value |
|---|---|
| Date | |
| Operator | |
| Ubuntu version | |
| Server ID | |
| CP URL | |
| Result | PASS / FAIL / PARTIAL |
| Notes (redacted) | |
