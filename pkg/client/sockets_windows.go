package client

import (
	"os"
	"path/filepath"
)

// ConnectorSocketName is the path used when communicating to the connector process
var ConnectorSocketName string

// DaemonSocketName is the path used when communicating to the daemon process
var DaemonSocketName string

func init() {
	ConnectorSocketName = filepath.Join(os.TempDir(), "telepresence-connector.socket")
	DaemonSocketName = filepath.Join(os.TempDir(), "telepresence-daemon.socket")
}
