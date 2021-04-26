package tun

// Name returns the name of this device, e.g. "tun0"
func (t *Device) Name() string {
	return t.name
}
