package protocol

// Standard platform network names
const (
	// ProxyNetworkName is the dedicated reverse-proxy Docker network (Traefik ingress).
	ProxyNetworkName = "deploycore-proxy"
)

// Network Label Keys
const (
	LabelNetworkType     = "deploycore.network.type"
	LabelNetworkName     = "deploycore.network.name"
	LabelProjectID       = "deploycore.project_id"
	LabelProjectSlug     = "deploycore.project_slug"
	LabelEnvironmentSlug = "deploycore.environment_slug"
)

// Network Types
const (
	NetworkTypePrivate  = "private"
	NetworkTypeProxy    = "proxy"
	NetworkTypeIsolated = "isolated"
)
