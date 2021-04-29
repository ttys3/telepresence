package tun

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/datawire/dlib/derror"

	"golang.org/x/sys/windows"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"

	"github.com/telepresenceio/telepresence/v2/pkg/tun/buffer"
)

type Device struct {
	tun.Device
	dev  *device.Device
	name string
}

// OpenTun creates a new TUN device and ensures that it is up and running.
func OpenTun(ctx context.Context) (td *Device, err error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			if err, ok = r.(error); !ok {
				err = derror.PanicToError(r)
			}
		}
	}()
	interfaceName := "tel0"
	td = &Device{}
	if td.Device, err = tun.CreateTUN(interfaceName, 0); err != nil {
		return nil, fmt.Errorf("failed to create TUN device: %v", err)
	}
	if td.name, err = td.Device.Name(); err != nil {
		return nil, fmt.Errorf("failed to get real name of TUN device: %v", err)
	}
	return td, nil
}

func (t *Device) getLUID() winipcfg.LUID {
	return winipcfg.LUID(t.Device.(*tun.NativeTun).LUID())
}

func (t *Device) AddSubnet(_ context.Context, subnet *net.IPNet) error {
	return t.getLUID().AddIPAddress(*subnet)
}

// RemoveSubnet removes a subnet from this TUN device and also removes the route for that subnet which
// is associated with the device.
func (t *Device) RemoveSubnet(_ context.Context, subnet *net.IPNet) error {
	return t.getLUID().DeleteIPAddress(*subnet)
}

func (t *Device) FlushDNSCache(_ context.Context) (err error) {
	return nil
}

func (t *Device) SetDNS(_ context.Context, server net.IP, domains []string) (err error) {
	luid := t.getLUID()
	if ip4 := server.To4(); ip4 != nil {
		return luid.SetDNS(windows.AF_INET, []net.IP{ip4}, domains)
	}
	return luid.SetDNS(windows.AF_INET6, []net.IP{server}, domains)
}

func (t *Device) SetMTU(mtu int) error {
	return errors.New("not implemented")
}

// Read reads as many bytes as possible into the given buffer.Data and returns the
// number of bytes actually read
func (t *Device) Read(into *buffer.Data) (int, error) {
	return t.Device.Read(into.Raw(), 0)
}

// Write writes bytes from the given buffer.Data and returns the number of bytes
// actually written.
func (t *Device) Write(from *buffer.Data) (int, error) {
	return t.Device.Write(from.Raw(), 0)
}
