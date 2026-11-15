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
	Multicast                // 224/4 and ff00::/8
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

// ClassOf returns the class of ip. An IPv4 address written as IPv6 (::ffff:127.0.0.1) is classed as the IPv4 address it holds.
func ClassOf(ip net.IP) Class {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	switch {
	case ip.Equal(metadataV4), ip.Equal(metadataV6):
		return Metadata
	case ip.IsUnspecified():
		return Unspecified
	case ip.IsLoopback():
		return Loopback
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast(), ip.IsInterfaceLocalMulticast():
		return LinkLocal
	case ip.IsMulticast():
		return Multicast
	case ip.IsPrivate(), sharedNAT.Contains(ip):
		return Private
	}
	return Public
}
