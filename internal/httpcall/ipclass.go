package httpcall

import "net"

// Class says what kind of address an IP is, for a policy that treats this machine and its network differently from the internet.
type Class int

const (
	Public      Class = iota // the internet
	Loopback                 // this machine: 127.0.0.0/8 and ::1
	Private                  // a local network: 10/8, 172.16/12, 192.168/16, fc00::/7, and the shared address space 100.64/10
	LinkLocal                // 169.254/16 and fe80::/10, which hold the cloud metadata address below
	Metadata                 // 169.254.169.254 and fd00:ec2::254, where cloud hosts give out credentials
	Unspecified              // 0.0.0.0 and ::, which reach this machine on many systems
	Multicast                // 224/4 and ff00::/8, every scope
	Reserved                 // ranges no public host answers from: 240/4, 255.255.255.255, 198.18/15, documentation and benchmarking ranges, fec0::/10, Teredo
)

func (c Class) String() string {
	switch c {
	case Loopback:
		return "this machine"
	case Private:
		return "a private network"
	case LinkLocal:
		return "a link-local address"
	case Metadata:
		return "a cloud metadata address"
	case Unspecified:
		return "an unspecified address"
	case Multicast:
		return "a multicast address"
	case Reserved:
		return "a reserved address"
	}
	return "the internet"
}

// Local says whether the class is anything but the internet: an address a person's own machine or network answers.
func (c Class) Local() bool { return c != Public }

var (
	metadataV4 = net.ParseIP("169.254.169.254")
	metadataV6 = net.ParseIP("fd00:ec2::254")
	sharedNAT  = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}
)

func cidr(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

var reservedNets = []*net.IPNet{
	cidr("240.0.0.0/4"), cidr("198.18.0.0/15"), cidr("192.0.0.0/24"), cidr("192.0.2.0/24"), cidr("198.51.100.0/24"),
	cidr("203.0.113.0/24"), cidr("192.88.99.0/24"), cidr("fec0::/10"), cidr("2001::/32"), cidr("2001:db8::/32"), cidr("100::/64"),
	cidr("64:ff9b:1::/48"), // local-use NAT64
}

var (
	nat64   = cidr("64:ff9b::/96")
	sixToFo = cidr("2002::/16")
)

// embedded returns the IPv4 address an IPv6 address carries, for the forms that route to it: NAT64 (64:ff9b::/96), 6to4 (2002::/16),
// IPv4-compatible (::a.b.c.d) and IPv4-translated (::ffff:0:a.b.c.d).
func embedded(ip net.IP) net.IP {
	b := ip.To16()
	if b == nil || ip.To4() != nil {
		return nil
	}
	switch {
	case nat64.Contains(ip):
		return net.IPv4(b[12], b[13], b[14], b[15]).To4()
	case sixToFo.Contains(ip):
		return net.IPv4(b[2], b[3], b[4], b[5]).To4()
	}
	zeros := true
	for _, x := range b[:12] {
		if x != 0 {
			zeros = false
		}
	}
	if zeros && !ip.IsUnspecified() && !ip.IsLoopback() {
		return net.IPv4(b[12], b[13], b[14], b[15]).To4()
	}
	// IPv4-translated is written 0:0:0:0:ffff:0:a.b.c.d: bytes 0-7 zero, 8-9 ff, 10-11 zero.
	if b[8] == 0xff && b[9] == 0xff && b[10] == 0 && b[11] == 0 && allZero(b[:8]) {
		return net.IPv4(b[12], b[13], b[14], b[15]).To4()
	}
	return nil
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

// ClassOf returns the class of ip. An IPv4 address written as IPv6 (::ffff:127.0.0.1), and an IPv6 address that routes to an IPv4 one
// (NAT64, 6to4, IPv4-compatible, IPv4-translated), is classed as the IPv4 address it holds.
func ClassOf(ip net.IP) Class {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	} else if v4 := embedded(ip); v4 != nil {
		ip = v4
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 0 {
		return Unspecified // 0.0.0.0/8, "this network"
	}
	switch {
	case ip.Equal(metadataV4), ip.Equal(metadataV6):
		return Metadata
	case ip.IsUnspecified():
		return Unspecified
	case ip.IsLoopback():
		return Loopback
	case ip.IsMulticast():
		return Multicast // all of it, also 224.0.0.0/24 and ff02::/16, which are link-local in scope
	case ip.IsLinkLocalUnicast():
		return LinkLocal
	case ip.IsPrivate(), sharedNAT.Contains(ip):
		return Private
	}
	if v4 := ip.To4(); v4 != nil && v4.Equal(net.IPv4bcast) {
		return Reserved
	}
	for _, n := range reservedNets {
		if n.Contains(ip) {
			return Reserved
		}
	}
	return Public
}
