package network

import (
	"fmt"
	"github.com/3th1nk/cidr"
)

// For now we'll just do huge hacks, future me can clean it up on a
// flight.
type UserNetwork struct {
	Method string
	Cidr   string
	Domain string
	Hosts  []StaticHost

	Libvirt string
}

type UserLibVirtNetwork struct {
}

type StaticHost struct {
	Name string
	Ip   string
}

type Network struct {
	Method string //TODO currently unused
	Cidr   cidr.CIDR
	Domain string
	Hosts  []Host //TODO make this a map

	Libvirt string
}

type Host struct {
	Fqdn string
	Name string
	Ipv4 string
}

// // Custom format function for debugging more simply.
// func (u Network) Format(f fmt.State, r rune) {
// 	out := fmt.Sprintf("unit %s: ", u.Name)
// 	if u.Kind != "" {
// 		out = fmt.Sprintf("%s kind: %s", out, u.Kind)
// 	}
// 	if len(u.After) > 0 {
// 		out = fmt.Sprintf("%s after: %s", out, u.After)
// 	}
// 	out = fmt.Sprintf("%s %s", out, u.Config)
// 	f.Write([]byte(out))
// }

func (un UserNetwork) ToNetwork() (n Network, e error) {
	userCidr, err := cidr.Parse(un.Cidr)
	if err != nil {
		return n, err
	}
	// TODO domain validation of some fashion?
	//
	// Probably should make sure the string maps to a valid
	// hostname and doesn't have like emoji or whatever.

	n.Cidr = *userCidr
	n.Domain = un.Domain
	var hosts []Host

	_, endIp := userCidr.IPRange()
	autoIp := endIp
	//	cidr.IPDecr(autoIp)

	for _, v := range un.Hosts {
		fqdn := fmt.Sprintf("%s.%s", v.Name, un.Domain)
		name := v.Name

		// When I add bridge mode will need to also handle
		// passing in ip addresses and to ensure auto ip's
		// don't collide somehow, might be easier to let
		// things fail.
		if v.Ip == "auto" {
			// Assign an ip from the cidr range
			//
			// For shits we'll assign from the end instead
			// of beginning to make it apparent.
			thisIp := autoIp
			cidr.IPDecr(autoIp)
			autoIp = thisIp
			hosts = append(hosts, Host{Fqdn: fqdn, Name: name, Ipv4: fmt.Sprintf("%s", thisIp)})
		} // TODO add checking/parsing of ip's to make sure they fit in the cidr
	}
	n.Hosts = hosts

	return n, nil
}
