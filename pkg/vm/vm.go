package vm

import (
	"crypto/rand"
	"fmt"
	"github.com/docker/go-units"
)

// Generic vm configuration, we need at the least the image to boot
// from, how big /dev/{s,v}da is, how big memory should be, and cpu
// count.
//
// All inputs for memory/disk are in strings to allow for stuff like
// 20g/100MiB etc...
//
// This is all then convertable into a Config struct which stores that
// junk into int64 byte values.
type UserConfig struct {
	Count    int // I'm not overly keen on this being here tbh
	Cpu      int
	Disksize string
	Memory   string
	Name     string
	Qcow2    string
}

type Config struct {
	Count    int // I'm not overly keen on this being here tbh
	Cpu      int
	Disksize int64
	Memory   int64
	Name     string
	Qcow2    string
}

const (
	// Mac address related constants aka if a mac is local or
	// multicast We only use local. Ref local bit
	// https://en.wikipedia.org/wiki/MAC_address
	LOCALMAC = 0b10
	MCASTMAC = 0b1

	// Sizing related nonsense
	GIB = (1024 * 1024 * 1024)
	MIB = (1024 * 1024)
	KIB = 1024
)

func fmtIec(bytes int64) string {
	// The dum units library doesn't work sanely at outputting in
	// MiB/KiB/GiB or letting you pick the output range, so just
	// do it all manually. Go is not my favorite language.
	if bytes >= GIB {
		//TODO should see if something is a multiple of 1024
		// and if so skip the .2 so that we get 20GiB vs
		// 20.00GiB. Not a huge problem just me being picky
		// af.
		return fmt.Sprintf("%.2fGiB", float64(bytes/GIB))
	}

	if bytes >= MIB {
		return fmt.Sprintf("%.2fMiB", float64(bytes/MIB))
	}

	if bytes >= KIB {
		return fmt.Sprintf("%.2fKiB", float64(bytes/KIB))
	}

	return fmt.Sprintf("%dbytes", bytes)
}

// Custom format function for debugging more simply.
func (c Config) Format(f fmt.State, r rune) {
	out := fmt.Sprintf("%s: count=%d cpu=%d disksize=%s mem=%s qcow2=%s", c.Name, c.Count, c.Cpu, fmtIec(c.Disksize), fmtIec(c.Memory), c.Qcow2)
	f.Write([]byte(out))
}

// Function to map across userconfig structs to a config struct
func (uc UserConfig) ToConfig() (c Config, e error) {
	bytes, err := units.RAMInBytes(uc.Disksize)
	if err != nil {
		return c, err
	}
	c.Disksize = bytes

	membytes, err := units.RAMInBytes(uc.Memory)
	if err != nil {
		return c, err
	}
	c.Memory = membytes

	c.Count = uc.Count
	c.Cpu = uc.Cpu
	c.Name = uc.Name
	c.Qcow2 = uc.Qcow2
	return c, nil
}

// Generate a random local ethernet mac address
func Randmac() (string, error) {
	// buf := make([]byte, 6)
	// _, err := rand.Read(buf)
	// if err != nil {
	// 	return "", err
	// }
	// // clear multicast bit (&^), ensure local bit (|)

	// buf[0] = buf[0]&^MCASTMAC | LOCALMAC
	// return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5]), nil

	buf := make([]byte, 3)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	// clear multicast bit (&^), ensure local bit (|)

	buf[0] = buf[0]&^MCASTMAC | LOCALMAC
	return fmt.Sprintf("C0:FF:EE:%02x:%02x:%02x", buf[0], buf[1], buf[2]), nil
}
