// Package ifcace deals with interfaces.
package iface

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// List returns a list of IP addresses from an interface.
func List(name string) ([]string, error) {
	ifis, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var addrs []net.Addr
	for _, ifi := range ifis {
		if ifi.Name == name {
			addrs, _ = ifi.Addrs()
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses found")
	}

	ips := []string{}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ipnet.IP.IsLinkLocalUnicast() || ipnet.IP.IsUnspecified() {
			continue
		}
		ips = append(ips, ipnet.IP.String())
	}
	return ips, nil
}

// Addr returns a list of routable IP addresses from an interface[:port] value. If no
// port is given, 53 is assumed. If you're using an interface alias you must specify an port number, i.e.
// "eth0:1:53", if only "eth0:1" is used you will try to listen on port 1.
func Addr(name string) ([]string, error) {
	if strings.HasSuffix(name, ":") {
		return nil, fmt.Errorf("name '%s' can not end in a colon", name)
	}
	i := strings.LastIndex(name, ":")
	port := "53"
	if i > 0 {
		port = name[i+1:]
		if _, err := strconv.Atoi(port); err != nil {
			return nil, fmt.Errorf("port '%s' isn't numerical: %w", port, err)
		}
		name = name[:i]
	}
	addrs, err := List(name)
	for i := range addrs {
		addrs[i] = net.JoinHostPort(addrs[i], port)
	}
	return addrs, err
}
