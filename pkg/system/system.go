package system

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// Runtime check for the release binary
func AmIRoot() (bool, error) {
	userCurrent, err := user.Current()
	if err != nil {
		return false, err
	}
	return (userCurrent.Username == "root"), nil
}

// Mimic of the install command. Take source file and copy it to
// destination with mode and owner/group specified.
//
// If anything it was doing was amiss it will fail.
//
// TODOAlso have this be idempotent and see if it needs to do work or
// not. For now it will always copy and chown data.
func Install(source string, destination string, mode int, osuser string, osgroup string) error {
	// User needs to have cwd set to something useful
	lhs, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	rhs, err := filepath.Abs(destination)
	if err != nil {
		return err
	}

	base := filepath.Dir(rhs)
	// For destination, create all dirctories above it, dirs will
	// always be 755 for sanity
	err = os.MkdirAll(base, os.FileMode(0755))
	if err != nil {
		return err
	}

	data, err := ioutil.ReadFile(lhs)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(rhs, data, os.FileMode(mode))
	if err != nil {
		return err
	}

	username, err := user.Lookup(osuser)
	if err != nil {
		return err
	}

	uid, _ := strconv.Atoi(username.Uid)

	groupname, err := user.LookupGroup(osgroup)
	if err != nil {
		return err
	}

	gid, _ := strconv.Atoi(groupname.Gid)

	err = syscall.Chown(rhs, uid, gid)

	return nil
}
