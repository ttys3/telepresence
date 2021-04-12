package iputil

import "net"

// Parse is like net.ParseIP but converts an IPv4 in 16 byte form to its 4 byte form
func Parse(ipStr string) (ip net.IP) {
	if ip = net.ParseIP(ipStr); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			ip = ip4
		}
	}
	return
}
