DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE organization_id IS NULL);

DELETE FROM roles WHERE organization_id IS NULL AND is_system = TRUE;

DELETE FROM permissions WHERE key IN (
    'organization.read', 'organization.update', 'organization.delete',
    'member.read', 'member.invite', 'member.update', 'member.remove',
    'server.read', 'server.create', 'server.update', 'server.delete',
    'project.read', 'project.create', 'project.update', 'project.delete',
    'application.read', 'application.create', 'application.update', 'application.deploy',
    'application.restart', 'application.stop', 'application.delete',
    'deployment.read', 'deployment.create', 'deployment.cancel', 'deployment.rollback',
    'secret.read_metadata', 'secret.create', 'secret.update', 'secret.delete',
    'database.read', 'database.create', 'database.update', 'database.backup', 'database.restore',
    'audit.read'
);

DROP TABLE IF EXISTS organization_invitation_roles;
DROP TABLE IF EXISTS organization_invitations;
