package daemon

import (
    "context"
    "io/ioutil"
    "net"
    "strings"

    "github.com/pkg/errors"

    "github.com/datawire/dlib/dgroup"
    "github.com/datawire/dlib/dlog"
    "github.com/telepresenceio/telepresence/v2/pkg/client/daemon/dns"
)

var errResolveDNotConfigured = errors.New("resolved not configured")

func (o *outbound) dnsServerWorker(c context.Context, onReady func()) error {
	err := o.tryResolveD(dgroup.WithGoroutineName(c, "/resolved"), onReady)
	if err == errResolveDNotConfigured {
		dlog.Info(c, "Unable to use systemd-resolved, falling back to local server")
		err = o.runOverridingServer(dgroup.WithGoroutineName(c, "/legacy"), onReady)
	}
	return err
}

func (o *outbound) runOverridingServer(c context.Context, onReady func()) error {
	if o.dnsIP == "" {
		dat, err := ioutil.ReadFile("/etc/resolv.conf")
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(dat), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "nameserver") {
				fields := strings.Fields(line)
				o.dnsIP = fields[1]
				dlog.Infof(c, "Automatically set -dns=%v", o.dnsIP)
				break
			}
		}
	}
	if o.dnsIP == "" {
		return errors.New("couldn't determine dns ip from /etc/resolv.conf")
	}

	if o.fallbackIP == "" {
		if o.dnsIP == "8.8.8.8" {
			o.fallbackIP = "8.8.4.4"
		} else {
			o.fallbackIP = "8.8.8.8"
		}
		dlog.Infof(c, "Automatically set -fallback=%v", o.fallbackIP)
	}
	if o.fallbackIP == o.dnsIP {
		return errors.New("if your fallbackIP and your dnsIP are the same, you will have a dns loop")
	}

	o.setSearchPathFunc = func(c context.Context, paths []string) {
		namespaces := make(map[string]struct{})
		search := make([]string, 0)
		for _, path := range paths {
			if strings.ContainsRune(path, '.') {
				search = append(search, path)
			} else if path != "" {
				namespaces[path] = struct{}{}
			}
		}
		namespaces[tel2SubDomain] = struct{}{}
		o.domainsLock.Lock()
		o.namespaces = namespaces
		o.search = search
		o.domainsLock.Unlock()
	}

	listeners, err := o.dnsListeners(c)
	if err != nil {
		return err
	}
	dnsAddr, err := splitToUDPAddr(listeners[0].LocalAddr())
	if err != nil {
		return err
	}
	o.dnsRedirPort = dnsAddr.Port

	// do not write iptables redirect
	o.overridePrimaryDNS = false

	onReady()

	// for local clash 127.0.0.1:5354
	o.fallbackIP = "127.0.0.1"
	srv := dns.NewServer(c, listeners, o.fallbackIP+":5354", o.resolveWithSearch)
	dlog.Debug(c, "Starting server")
	err = srv.Run(c)
	dlog.Debug(c, "Server done")
	return err
}

// resolveWithSearch looks up the given query and returns the matching IPs.
//
// Queries using qualified names will be dispatched to the resolveNoSearch() function.
// An unqualified name query will be tried with all the suffixes in the search path
// and the IPs of the first match will be returned.
func (o *outbound) resolveWithSearch(query string) []string {
	if strings.Count(query, ".") > 1 {
		// More than just the ending dot, so don't use search-path
		return o.resolveNoSearch(query)
	}
	o.domainsLock.RLock()
	ips := o.resolveWithSearchLocked(strings.ToLower(query))
	o.domainsLock.RUnlock()
	return ips
}

// listen to 127.0.0.53:53 like systemd-resolved
func (o *outbound) dnsListeners(c context.Context) ([]net.PacketConn, error) {
	listeners := []net.PacketConn{o.dnsListener}

    if ls, err := net.ListenPacket("udp", "127.0.0.53:53"); err == nil {
	if _, err = splitToUDPAddr(ls.LocalAddr()); err != nil {
	    ls.Close()
	    return nil, err
	}
	_ = listeners[0].Close()
	listeners = []net.PacketConn{ls}
    }
    return listeners, nil
}
