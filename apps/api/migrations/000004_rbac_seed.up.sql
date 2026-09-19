-- Phase B4: invitations table + seed system RBAC catalog.

CREATE TABLE organization_invitations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email            TEXT NOT NULL,
    invited_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    token_hash       TEXT NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    accepted_at      TIMESTAMPTZ,
    revoked_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT organization_invitations_token_hash_uidx UNIQUE (token_hash)
);

CREATE UNIQUE INDEX organization_invitations_org_email_open_uidx
    ON organization_invitations (organization_id, LOWER(email))
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

CREATE INDEX organization_invitations_organization_id_idx
    ON organization_invitations (organization_id);

CREATE TABLE organization_invitation_roles (
    invitation_id UUID NOT NULL REFERENCES organization_invitations (id) ON DELETE CASCADE,
    role_id       UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (invitation_id, role_id)
);

-- ---------------------------------------------------------------------------
-- Permissions catalog
-- ---------------------------------------------------------------------------

INSERT INTO permissions (key, description) VALUES
    ('organization.read', 'View organization details'),
    ('organization.update', 'Update organization settings'),
    ('organization.delete', 'Delete or schedule deletion of an organization'),
    ('member.read', 'List organization members'),
    ('member.invite', 'Invite members to the organization'),
    ('member.update', 'Change member roles'),
    ('member.remove', 'Remove members from the organization'),
    ('server.read', 'View servers'),
    ('server.create', 'Register servers'),
    ('server.update', 'Update servers'),
    ('server.delete', 'Delete servers'),
    ('project.read', 'View projects'),
    ('project.create', 'Create projects'),
    ('project.update', 'Update projects'),
    ('project.delete', 'Delete projects'),
    ('application.read', 'View applications'),
    ('application.create', 'Create applications'),
    ('application.update', 'Update applications'),
    ('application.deploy', 'Deploy applications'),
    ('application.restart', 'Restart applications'),
    ('application.stop', 'Stop applications'),
    ('application.delete', 'Delete applications'),
    ('deployment.read', 'View deployments'),
    ('deployment.create', 'Create deployments'),
    ('deployment.cancel', 'Cancel deployments'),
    ('deployment.rollback', 'Rollback deployments'),
    ('secret.read_metadata', 'View secret metadata'),
    ('secret.create', 'Create secrets'),
    ('secret.update', 'Update secrets'),
    ('secret.delete', 'Delete secrets'),
    ('database.read', 'View managed databases'),
    ('database.create', 'Create managed databases'),
    ('database.update', 'Update managed databases'),
    ('database.backup', 'Create database backups'),
    ('database.restore', 'Restore database backups'),
    ('audit.read', 'View audit logs')
ON CONFLICT (key) DO NOTHING;

-- ---------------------------------------------------------------------------
-- System roles
-- ---------------------------------------------------------------------------

INSERT INTO roles (key, name, description, is_system)
SELECT v.key, v.name, v.description, TRUE
FROM (VALUES
    ('owner', 'Owner', 'Full control of the organization, billing, and membership.'),
    ('administrator', 'Administrator', 'Manage infrastructure and applications; limited org settings.'),
    ('devops', 'DevOps', 'Operate servers, deployments, databases, and backups.'),
    ('developer', 'Developer', 'Deploy and configure applications within assigned projects.'),
    ('support', 'Support', 'Read-mostly access for troubleshooting production issues.'),
    ('viewer', 'Viewer', 'Read-only visibility into projects and runtime status.')
) AS v(key, name, description)
WHERE NOT EXISTS (
    SELECT 1 FROM roles r WHERE r.organization_id IS NULL AND r.key = v.key
);

-- Owner: every permission
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.organization_id IS NULL AND r.key = 'owner'
ON CONFLICT DO NOTHING;

-- Administrator: all except organization.delete
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.organization_id IS NULL
  AND r.key = 'administrator'
  AND p.key <> 'organization.delete'
ON CONFLICT DO NOTHING;

-- DevOps
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.key IN (
    'organization.read',
    'member.read',
    'server.read', 'server.create', 'server.update', 'server.delete',
    'project.read', 'project.create', 'project.update', 'project.delete',
    'application.read', 'application.create', 'application.update', 'application.deploy',
    'application.restart', 'application.stop', 'application.delete',
    'deployment.read', 'deployment.create', 'deployment.cancel', 'deployment.rollback',
    'secret.read_metadata', 'secret.create', 'secret.update', 'secret.delete',
    'database.read', 'database.create', 'database.update', 'database.backup', 'database.restore',
    'audit.read'
)
WHERE r.organization_id IS NULL AND r.key = 'devops'
ON CONFLICT DO NOTHING;

-- Developer
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.key IN (
    'organization.read',
    'member.read',
    'server.read',
    'project.read',
    'application.read', 'application.create', 'application.update', 'application.deploy',
    'application.restart', 'application.stop',
    'deployment.read', 'deployment.create', 'deployment.cancel', 'deployment.rollback',
    'secret.read_metadata', 'secret.create', 'secret.update',
    'database.read',
    'audit.read'
)
WHERE r.organization_id IS NULL AND r.key = 'developer'
ON CONFLICT DO NOTHING;

-- Support
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.key IN (
    'organization.read',
    'member.read',
    'server.read',
    'project.read',
    'application.read', 'application.restart',
    'deployment.read', 'deployment.cancel',
    'secret.read_metadata',
    'database.read',
    'audit.read'
)
WHERE r.organization_id IS NULL AND r.key = 'support'
ON CONFLICT DO NOTHING;

-- Viewer
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.key IN (
    'organization.read',
    'member.read',
    'server.read',
    'project.read',
    'application.read',
    'deployment.read',
    'secret.read_metadata',
    'database.read',
    'audit.read'
)
WHERE r.organization_id IS NULL AND r.key = 'viewer'
ON CONFLICT DO NOTHING;
