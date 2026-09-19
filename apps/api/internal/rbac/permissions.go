package rbac

// Permission keys enforced server-side.
const (
	OrganizationRead   = "organization.read"
	OrganizationUpdate = "organization.update"
	OrganizationDelete = "organization.delete"

	MemberRead   = "member.read"
	MemberInvite = "member.invite"
	MemberUpdate = "member.update"
	MemberRemove = "member.remove"

	ServerRead   = "server.read"
	ServerCreate = "server.create"
	ServerUpdate = "server.update"
	ServerDelete = "server.delete"

	ProjectRead   = "project.read"
	ProjectCreate = "project.create"
	ProjectUpdate = "project.update"
	ProjectDelete = "project.delete"

	ApplicationRead    = "application.read"
	ApplicationCreate  = "application.create"
	ApplicationUpdate  = "application.update"
	ApplicationDeploy  = "application.deploy"
	ApplicationRestart = "application.restart"
	ApplicationStop    = "application.stop"
	ApplicationDelete  = "application.delete"

	DeploymentRead     = "deployment.read"
	DeploymentCreate   = "deployment.create"
	DeploymentCancel   = "deployment.cancel"
	DeploymentRollback = "deployment.rollback"

	GitConnectionRead   = "git.connection.read"
	GitConnectionManage = "git.connection.manage"

	RegistryRead   = "registry.read"
	RegistryManage = "registry.manage"

	DomainRead   = "domain.read"
	DomainManage = "domain.manage"

	SecretReadMetadata = "secret.read_metadata"
	SecretCreate       = "secret.create"
	SecretUpdate       = "secret.update"
	SecretDelete       = "secret.delete"

	DatabaseRead    = "database.read"
	DatabaseCreate  = "database.create"
	DatabaseUpdate  = "database.update"
	DatabaseBackup  = "database.backup"
	DatabaseRestore = "database.restore"

	NotificationRead   = "notification.read"
	NotificationManage = "notification.manage"

	WebhookRead   = "webhook.read"
	WebhookManage = "webhook.manage"

	AuditRead = "audit.read"
)

// System role keys.
const (
	RoleOwner         = "owner"
	RoleAdministrator = "administrator"
	RoleDevOps        = "devops"
	RoleDeveloper     = "developer"
	RoleSupport       = "support"
	RoleViewer        = "viewer"
)
