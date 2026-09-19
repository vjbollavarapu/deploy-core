package rbac

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureSeeded upserts the system permission catalog and default roles.
// Safe to call on every process start; does not remove custom org roles.
func EnsureSeeded(ctx context.Context, pool *pgxpool.Pool) error {
	perms := []struct{ Key, Desc string }{
		{OrganizationRead, "View organization details"},
		{OrganizationUpdate, "Update organization settings"},
		{OrganizationDelete, "Delete or schedule deletion of an organization"},
		{MemberRead, "List organization members"},
		{MemberInvite, "Invite members to the organization"},
		{MemberUpdate, "Change member roles"},
		{MemberRemove, "Remove members from the organization"},
		{ServerRead, "View servers"},
		{ServerCreate, "Register servers"},
		{ServerUpdate, "Update servers"},
		{ServerDelete, "Delete servers"},
		{ProjectRead, "View projects"},
		{ProjectCreate, "Create projects"},
		{ProjectUpdate, "Update projects"},
		{ProjectDelete, "Delete projects"},
		{ApplicationRead, "View applications"},
		{ApplicationCreate, "Create applications"},
		{ApplicationUpdate, "Update applications"},
		{ApplicationDeploy, "Deploy applications"},
		{ApplicationRestart, "Restart applications"},
		{ApplicationStop, "Stop applications"},
		{ApplicationDelete, "Delete applications"},
		{DeploymentRead, "View deployments"},
		{DeploymentCreate, "Create deployments"},
		{DeploymentCancel, "Cancel deployments"},
		{DeploymentRollback, "Rollback deployments"},
		{GitConnectionRead, "View git provider connections"},
		{GitConnectionManage, "Manage git provider connections"},
		{RegistryRead, "View container registries"},
		{RegistryManage, "Manage container registries"},
		{DomainRead, "View application domains"},
		{DomainManage, "Manage application domains and routing"},
		{SecretReadMetadata, "View secret metadata"},
		{SecretCreate, "Create secrets"},
		{SecretUpdate, "Update secrets"},
		{SecretDelete, "Delete secrets"},
		{DatabaseRead, "View managed databases"},
		{DatabaseCreate, "Create managed databases"},
		{DatabaseUpdate, "Update managed databases"},
		{DatabaseBackup, "Create database backups"},
		{DatabaseRestore, "Restore database backups"},
		{NotificationRead, "View notification channels, policies, and deliveries"},
		{NotificationManage, "Manage notification channels and policies"},
		{WebhookRead, "View outgoing webhooks and deliveries"},
		{WebhookManage, "Manage outgoing webhooks"},
		{AuditRead, "View audit logs"},
	}
	for _, p := range perms {
		if _, err := pool.Exec(ctx, `
			INSERT INTO permissions (key, description) VALUES ($1, $2)
			ON CONFLICT (key) DO NOTHING`, p.Key, p.Desc); err != nil {
			return fmt.Errorf("seed permission %s: %w", p.Key, err)
		}
	}

	roles := []struct{ Key, Name, Desc string }{
		{RoleOwner, "Owner", "Full control of the organization, billing, and membership."},
		{RoleAdministrator, "Administrator", "Manage infrastructure and applications; limited org settings."},
		{RoleDevOps, "DevOps", "Operate servers, deployments, databases, and backups."},
		{RoleDeveloper, "Developer", "Deploy and configure applications within assigned projects."},
		{RoleSupport, "Support", "Read-mostly access for troubleshooting production issues."},
		{RoleViewer, "Viewer", "Read-only visibility into projects and runtime status."},
	}
	for _, role := range roles {
		if _, err := pool.Exec(ctx, `
			INSERT INTO roles (key, name, description, is_system)
			SELECT $1, $2, $3, TRUE
			WHERE NOT EXISTS (
				SELECT 1 FROM roles r WHERE r.organization_id IS NULL AND r.key = $1
			)`, role.Key, role.Name, role.Desc); err != nil {
			return fmt.Errorf("seed role %s: %w", role.Key, err)
		}
	}

	type rolePerms struct {
		Role string
		Keys []string
	}
	allKeys := make([]string, 0, len(perms))
	for _, p := range perms {
		allKeys = append(allKeys, p.Key)
	}
	adminKeys := make([]string, 0, len(allKeys))
	for _, k := range allKeys {
		if k != OrganizationDelete {
			adminKeys = append(adminKeys, k)
		}
	}
	matrix := []rolePerms{
		{RoleOwner, allKeys},
		{RoleAdministrator, adminKeys},
		{RoleDevOps, []string{
			OrganizationRead, MemberRead,
			ServerRead, ServerCreate, ServerUpdate, ServerDelete,
			ProjectRead, ProjectCreate, ProjectUpdate, ProjectDelete,
			ApplicationRead, ApplicationCreate, ApplicationUpdate, ApplicationDeploy,
			ApplicationRestart, ApplicationStop, ApplicationDelete,
			DeploymentRead, DeploymentCreate, DeploymentCancel, DeploymentRollback,
			GitConnectionRead, GitConnectionManage,
			RegistryRead, RegistryManage,
			DomainRead, DomainManage,
			SecretReadMetadata, SecretCreate, SecretUpdate, SecretDelete,
			DatabaseRead, DatabaseCreate, DatabaseUpdate, DatabaseBackup, DatabaseRestore,
			NotificationRead, NotificationManage,
			WebhookRead, WebhookManage,
			AuditRead,
		}},
		{RoleDeveloper, []string{
			OrganizationRead, MemberRead, ServerRead, ProjectRead,
			ApplicationRead, ApplicationCreate, ApplicationUpdate, ApplicationDeploy,
			ApplicationRestart, ApplicationStop,
			DeploymentRead, DeploymentCreate, DeploymentCancel, DeploymentRollback,
			GitConnectionRead, GitConnectionManage,
			RegistryRead, RegistryManage,
			DomainRead, DomainManage,
			SecretReadMetadata, SecretCreate, SecretUpdate,
			DatabaseRead, NotificationRead, WebhookRead, AuditRead,
		}},
		{RoleSupport, []string{
			OrganizationRead, MemberRead, ServerRead, ProjectRead,
			ApplicationRead, ApplicationRestart,
			DeploymentRead, DeploymentCancel,
			SecretReadMetadata, DatabaseRead, NotificationRead, WebhookRead, AuditRead,
			GitConnectionRead, RegistryRead, DomainRead,
		}},
		{RoleViewer, []string{
			OrganizationRead, MemberRead, ServerRead, ProjectRead,
			ApplicationRead, DeploymentRead, SecretReadMetadata, DatabaseRead, NotificationRead, WebhookRead, AuditRead,
			GitConnectionRead, RegistryRead, DomainRead,
		}},
	}

	for _, rp := range matrix {
		for _, key := range rp.Keys {
			if _, err := pool.Exec(ctx, `
				INSERT INTO role_permissions (role_id, permission_id)
				SELECT r.id, p.id
				FROM roles r
				JOIN permissions p ON p.key = $2
				WHERE r.organization_id IS NULL AND r.key = $1
				ON CONFLICT DO NOTHING`, rp.Role, key); err != nil {
				return fmt.Errorf("seed role_permission %s/%s: %w", rp.Role, key, err)
			}
		}
	}
	return nil
}
