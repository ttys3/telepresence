// +build !windows

package client

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/telepresenceio/telepresence/v2/pkg/proc"

	"google.golang.org/grpc"
)

const (
	// ConnectorSocketName is the path used when communicating to the connector process
	ConnectorSocketName = "/tmp/telepresence-connector.socket"

	// DaemonSocketName is the path used when communicating to the daemon process
	DaemonSocketName = "/var/run/telepresence-daemon.socket"
)

// DialSocket dials the given unix socket and returns the resulting connection
func DialSocket(c context.Context, socketName string) (*grpc.ClientConn, error) {
	return grpc.DialContext(c, "unix:"+socketName, grpc.WithInsecure(), grpc.WithNoProxy())
}

// ListenSocket returns a listener for the given named pipe and returns the resulting connection
func ListenSocket(_ context.Context, socketName string) (net.Listener, error) {
	l, err := net.Listen("unix", socketName)
	if err != nil {
		return nil, err
	}
	if proc.IsAdmin() {
		err = os.Chmod(client.DaemonSocketName, 0777)
		if err != nil {
			return fmt.Errorf("chmod on socket %s: %v", socketName)
		}
	}
	return l, nil
}

// SocketExists returns true if a socket is found at the given path
func SocketExists(path string) bool {
	s, err := os.Stat(path)
	return err == nil && s.Mode()&os.ModeSocket != 0
}
