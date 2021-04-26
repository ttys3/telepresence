package tun

import (
	"context"
	"errors"
	"net"
	"os"

	"github.com/telepresenceio/telepresence/v2/pkg/tun/buffer"
)

type Device struct {
	*os.File
	name string
}

// OpenTun creates a new TUN device and ensures that it is up and running.
func OpenTun() (*Device, error) {
	return nil, errors.New("not implemented")
}

func (t *Device) AddSubnet(ctx context.Context, subnet *net.IPNet) error {
	return errors.New("not implemented")
}

// RemoveSubnet removes a subnet from this TUN device and also removes the route for that subnet which
// is associated with the device.
func (t *Device) RemoveSubnet(ctx context.Context, subnet *net.IPNet) error {
	return errors.New("not implemented")
}

func (t *Device) SetMTU(mtu int) error {
	return errors.New("not implemented")
}

// Read reads as many bytes as possible into the given buffer.Data and returns the
// number of bytes actually read
func (t *Device) Read(into *buffer.Data) (int, error) {
	return t.File.Read(into.Raw())
}

// Write writes bytes from the given buffer.Data and returns the number of bytes
// actually written.
func (t *Device) Write(from *buffer.Data) (int, error) {
	return t.File.Write(from.Raw())
}
