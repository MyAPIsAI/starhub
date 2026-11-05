package config

import (
	"errors"
	"net/netip"
	"net/url"
	"starhub/common"
	"strings"
	"sync/atomic"
)

var staffNetworks atomic.Pointer[[]netip.Prefix]
var staffOpenSource atomic.Bool

func StaffOpenSourceSetup() bool { return staffOpenSource.Load() }
func ConfigureStaffSetup(mode string) error {
	if mode != "" && mode != "commercial" && mode != "opensource" {
		return errors.New("staff setup mode must be commercial or opensource")
	}
	staffOpenSource.Store(mode == "opensource")
	return nil
}

type SnapshotSource struct{ URL, Token string }

var snapshotSource atomic.Pointer[SnapshotSource]

func ConfigureSnapshotSource(address, token string) error {
	if address != "" {
		parsed, err := url.Parse(address)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
			return errors.New("invalid resource snapshot source")
		}
		if parsed.Scheme != "https" {
			ip, err := netip.ParseAddr(parsed.Hostname())
			if parsed.Scheme != "http" || (parsed.Hostname() != "localhost" && (err != nil || !ip.IsLoopback())) {
				return errors.New("resource snapshot source requires HTTPS outside loopback")
			}
		}
		if strings.ContainsAny(token, "\r\n") {
			return errors.New("invalid resource snapshot credential")
		}
	}
	snapshotSource.Store(&SnapshotSource{address, token})
	return nil
}

func ResourceSnapshotSource() SnapshotSource {
	if source := snapshotSource.Load(); source != nil {
		return *source
	}
	return SnapshotSource{}
}

func ConfigureStaffNetworks(values []string) error {
	list := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := common.ParseIPNetwork(strings.TrimSpace(value))
		if err != nil {
			return err
		}
		list = append(list, prefix.Masked())
	}
	staffNetworks.Store(&list)
	return nil
}

// Local development is loopback-only unless deployment declares company ranges.
func StaffIPAllowed(value string) bool {
	ip, err := netip.ParseAddr(value)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	list := staffNetworks.Load()
	if list == nil {
		return ip.IsLoopback()
	}
	for _, network := range *list {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
