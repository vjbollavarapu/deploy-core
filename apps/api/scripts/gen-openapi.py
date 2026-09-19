#!/usr/bin/env python3
"""Regenerate OpenAPI JSON + frontend contract types (Phase B32).

Usage (from repo root):
  python3 apps/api/scripts/gen-openapi.py

Writes:
  apps/api/docs/openapi.json
  apps/api/internal/openapi/openapi.json  (embed copy)
  apps/frontend/lib/api/contract.ts
  apps/api/docs/API.md (conventions stub refreshed lightly)
"""
from __future__ import annotations

import json
import pathlib
import re
import shutil
import textwrap

ROOT = pathlib.Path(__file__).resolve().parents[3]
API = ROOT / "apps" / "api"
FRONTEND = ROOT / "apps" / "frontend"


def collect_routes() -> list[tuple[str, str]]:
    routes: list[tuple[str, str]] = []
    for p in (API / "internal").rglob("handler.go"):
        text = p.read_text()
        for m in re.finditer(
            r'mux\.Handle(?:Func)?\("(GET|POST|PUT|PATCH|DELETE) ([^"]+)"', text
        ):
            routes.append((m.group(1), m.group(2)))
    routes.extend(
        [("GET", "/health"), ("GET", "/ready"), ("GET", "/openapi.json")]
    )
    return sorted(set(routes), key=lambda x: (x[1], x[0]))


def tag_for(path: str) -> str:
    if path.startswith("/auth"):
        return "Auth"
    if path.startswith("/organizations") or path.startswith("/invitations"):
        return "Organizations"
    if path.startswith("/projects") or path.startswith("/environments"):
        return "Projects"
    if path.startswith("/servers") or path.startswith("/placement") or path.startswith(
        "/commands"
    ):
        return "Servers"
    if path.startswith("/agents"):
        return "Agents"
    if path.startswith("/applications"):
        return "Applications"
    if path.startswith("/deployments") or path.startswith("/revisions"):
        return "Deployments"
    if path.startswith("/variables") or path.startswith("/secrets"):
        return "Config"
    if path.startswith("/integrations/git") or path.startswith("/webhooks/git"):
        return "Git"
    if path.startswith("/integrations/registries"):
        return "Registries"
    if path.startswith("/integrations/notifications"):
        return "Notifications"
    if path.startswith("/integrations/webhooks"):
        return "Webhooks"
    if path.startswith("/domains"):
        return "Domains"
    if (
        path.startswith("/databases")
        or path.startswith("/backups")
        or path.startswith("/restores")
    ):
        return "Databases"
    if path.startswith("/volumes"):
        return "Volumes"
    if path.startswith("/audit-logs"):
        return "Audit"
    if path in ("/health", "/ready", "/openapi.json"):
        return "System"
    return "Other"


def op_id(method: str, path: str) -> str:
    parts = [p for p in path.strip("/").split("/") if p]
    segs = []
    for p in parts:
        if p.startswith("{"):
            segs.append(
                "By"
                + "".join(
                    w.capitalize() for w in p[1:-1].replace("-", "_").split("_")
                )
            )
        else:
            segs.append(
                "".join(w.capitalize() for w in p.replace("-", "_").split("_"))
            )
    return method.lower() + "".join(segs)


def security_for(path: str) -> list:
    if path in ("/health", "/ready", "/openapi.json"):
        return []
    if path.startswith("/auth/") and path != "/auth/me":
        return []
    if path == "/agents/register" or path.startswith("/webhooks/git"):
        return []
    if path.startswith("/agents/"):
        return [{"AgentBearer": []}]
    return [{"BearerAuth": []}]


def build_spec(routes: list[tuple[str, str]]) -> dict:
    paths: dict = {}
    for method, path in routes:
        oas_path = path if path.startswith("/") else "/" + path
        full = (
            oas_path
            if oas_path in ("/health", "/ready", "/openapi.json")
            else "/api/v1" + oas_path
        )
        item = paths.setdefault(full, {})
        params = []
        for name in re.findall(r"\{([^}]+)\}", oas_path):
            params.append(
                {
                    "name": name,
                    "in": "path",
                    "required": True,
                    "schema": {"type": "string", "format": "uuid"},
                }
            )
        listish = (
            method == "GET"
            and "{" not in oas_path.split("/")[-1]
            and oas_path
            not in (
                "/health",
                "/ready",
                "/auth/me",
                "/openapi.json",
                "/variables/resolved",
                "/servers/capacity",
                "/integrations/notifications/meta",
                "/integrations/webhooks/meta",
            )
        )
        if listish:
            params.extend(
                [
                    {
                        "name": "limit",
                        "in": "query",
                        "schema": {
                            "type": "integer",
                            "minimum": 1,
                            "maximum": 100,
                            "default": 20,
                        },
                    },
                    {
                        "name": "offset",
                        "in": "query",
                        "schema": {"type": "integer", "minimum": 0, "default": 0},
                    },
                ]
            )
        responses = {
            "200": {
                "description": "Success",
                "content": {
                    "application/json": {
                        "schema": {"$ref": "#/components/schemas/JSONObject"}
                    }
                },
            },
            "default": {
                "description": "Error",
                "content": {
                    "application/json": {
                        "schema": {"$ref": "#/components/schemas/ErrorEnvelope"}
                    }
                },
            },
        }
        if method == "POST" and oas_path in (
            "/auth/register",
            "/agents/register",
            "/applications",
            "/projects",
            "/servers",
            "/organizations",
        ):
            responses = {
                "201": {
                    "description": "Created",
                    "content": {
                        "application/json": {
                            "schema": {"$ref": "#/components/schemas/JSONObject"}
                        }
                    },
                },
                "default": responses["default"],
            }
        if oas_path in ("/auth/login", "/auth/refresh", "/auth/register"):
            code = "201" if oas_path.endswith("register") else "200"
            responses = {
                code: {
                    "description": "Auth result",
                    "content": {
                        "application/json": {
                            "schema": {"$ref": "#/components/schemas/AuthResult"}
                        }
                    },
                },
                "default": responses["default"],
            }
        op = {
            "operationId": op_id(method, oas_path),
            "tags": [tag_for(oas_path)],
            "summary": f"{method} {oas_path}",
            "parameters": params,
            "responses": responses,
            "security": security_for(oas_path),
        }
        if method in ("POST", "PATCH", "PUT") and not oas_path.endswith(
            ("/registration-token", "/maintenance")
        ):
            schema_ref = "#/components/schemas/JSONObject"
            mapping = {
                "/auth/register": "#/components/schemas/RegisterRequest",
                "/auth/login": "#/components/schemas/LoginRequest",
                "/auth/refresh": "#/components/schemas/RefreshRequest",
                "/applications": "#/components/schemas/CreateApplicationRequest",
                "/projects": "#/components/schemas/CreateProjectRequest",
                "/servers": "#/components/schemas/CreateServerRequest",
            }
            if oas_path in mapping:
                schema_ref = mapping[oas_path]
            elif oas_path.endswith("/deployments"):
                schema_ref = "#/components/schemas/CreateDeploymentRequest"
            elif oas_path.endswith("/rollback"):
                schema_ref = "#/components/schemas/RollbackRequest"
            elif oas_path.endswith("/replicas/scale"):
                schema_ref = "#/components/schemas/ScaleReplicasRequest"
            elif oas_path.endswith("/logout"):
                schema_ref = "#/components/schemas/LogoutRequest"
            op["requestBody"] = {
                "required": oas_path != "/auth/logout",
                "content": {
                    "application/json": {"schema": {"$ref": schema_ref}}
                },
            }
        item[method.lower()] = op

    # Load schemas from existing file if present to preserve curated components.
    existing = API / "docs" / "openapi.json"
    components = json.loads(existing.read_text())["components"] if existing.exists() else {}
    return {
        "openapi": "3.1.0",
        "info": {
            "title": "DeployCore Control Plane API",
            "version": "0.32.0",
            "description": textwrap.dedent(
                """
                Public/control-plane HTTP API for DeployCore (`apps/api`).
                Base path: `/api/v1`. Spec: `GET /openapi.json`.
                """
            ).strip(),
        },
        "servers": [{"url": "/", "description": "Current host"}],
        "tags": [
            {"name": n}
            for n in [
                "System",
                "Auth",
                "Organizations",
                "Projects",
                "Servers",
                "Agents",
                "Applications",
                "Deployments",
                "Config",
                "Git",
                "Registries",
                "Domains",
                "Databases",
                "Volumes",
                "Notifications",
                "Webhooks",
                "Audit",
            ]
        ],
        "paths": paths,
        "components": components,
    }


def write_contract(spec: dict) -> None:
    schemas = spec["components"]["schemas"]
    lines = [
        "/**",
        " * DeployCore control-plane API contract types (B32).",
        " *",
        " * Sourced from apps/api/docs/openapi.json — wire-format shapes only.",
        " * Do not couple UI view-models in lib/types.ts to DB rows or Go structs.",
        " * Prefer mapping API → UI types at the client boundary.",
        " */",
        "",
        "export const API_BASE = '/api/v1' as const",
        "",
        "/** Wire-format UUID string (not a branded runtime type). */",
        "export type UUID = string",
        "",
        "/** RFC3339 timestamp string. */",
        "export type Timestamp = string",
        "",
        "export type JSONObject = Record<string, unknown>",
        "",
    ]

    def ts_type(schema: dict) -> str:
        if "$ref" in schema:
            return schema["$ref"].split("/")[-1]
        t = schema.get("type")
        if "enum" in schema:
            return " | ".join(json.dumps(x) for x in schema["enum"])
        if t == "string":
            return "string"
        if t in ("integer", "number"):
            return "number"
        if t == "boolean":
            return "boolean"
        if t == "array":
            return f"Array<{ts_type(schema.get('items', {}))}>"
        if t == "object" or "additionalProperties" in schema or "properties" in schema:
            ap = schema.get("additionalProperties")
            if ap is True:
                return "Record<string, unknown>"
            if isinstance(ap, dict):
                return f"Record<string, {ts_type(ap)}>"
            props = schema.get("properties") or {}
            if not props:
                return "Record<string, unknown>"
            req = set(schema.get("required") or [])
            parts = []
            for k, v in props.items():
                opt = "" if k in req else "?"
                tt = ts_type(v)
                if v.get("nullable"):
                    tt = f"{tt} | null"
                parts.append(f"  {k}{opt}: {tt}")
            return "{\n" + "\n".join(parts) + "\n}"
        return "unknown"

    for name, schema in schemas.items():
        if "enum" in schema:
            lines.append(f"export type {name} = {ts_type(schema)}")
            lines.append("")

    for name in [
        "APIError",
        "ErrorEnvelope",
        "TokenPair",
        "User",
        "AuthResult",
        "RegisterRequest",
        "LoginRequest",
        "RefreshRequest",
        "LogoutRequest",
        "CreateProjectRequest",
        "CreateServerRequest",
        "ApplicationConfigInput",
        "CreateApplicationRequest",
        "CreateDeploymentRequest",
        "RollbackRequest",
        "ScaleReplicasRequest",
        "Organization",
        "Server",
        "Application",
        "Deployment",
    ]:
        sch = schemas.get(name)
        if not sch or "enum" in sch:
            continue
        body = ts_type(sch)
        lines.append(f"export interface {name} {body}")
        lines.append("")

    lines.extend(
        [
            "/** Offset pagination query used by list endpoints. */",
            "export interface PageParams {",
            "  limit?: number",
            "  offset?: number",
            "}",
            "",
            "export interface Page<T> {",
            "  items: T[]",
            "  limit: number",
            "  offset: number",
            "  totalCount?: number | null",
            "}",
            "",
            "/** Permission keys enforced by the control plane (org-scoped). */",
            "export type PermissionKey = Permission",
            "",
        ]
    )
    out = FRONTEND / "lib" / "api" / "contract.ts"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text("\n".join(lines) + "\n")


def main() -> None:
    routes = collect_routes()
    spec = build_spec(routes)
    docs = API / "docs"
    docs.mkdir(parents=True, exist_ok=True)
    out = docs / "openapi.json"
    out.write_text(json.dumps(spec, indent=2) + "\n")
    embed = API / "internal" / "openapi" / "openapi.json"
    shutil.copyfile(out, embed)
    write_contract(spec)
    print(f"routes={len(routes)} paths={len(spec['paths'])} -> {out}")


if __name__ == "__main__":
    main()
