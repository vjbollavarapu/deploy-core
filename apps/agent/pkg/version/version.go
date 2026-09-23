package version

import (
	"fmt"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// Version is the agent version. Injectable via ldflags.
	Version = "0.0.0-dev"
	// BuildCommit is the git commit hash. Injectable via ldflags.
	BuildCommit = "unknown"
	// BuildDate is the build date. Injectable via ldflags.
	BuildDate = "unknown"
)

// Info holds version metadata for the agent.
type Info struct {
	Version       string
	BuildCommit   string
	BuildDate     string
	ProtocolMajor int
}

// Get returns the current version metadata.
func Get() Info {
	return Info{
		Version:       Version,
		BuildCommit:   BuildCommit,
		BuildDate:     BuildDate,
		ProtocolMajor: protocol.SchemaVersion,
	}
}

func (i Info) String() string {
	return fmt.Sprintf("deploycore-agent v%s (commit: %s, date: %s, protocol: v%d)", i.Version, i.BuildCommit, i.BuildDate, i.ProtocolMajor)
}
