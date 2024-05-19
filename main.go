package main

import (
	"log"
	"os"

	"deploy/pkg/cmd/local"
	"deploy/pkg/cmd/pulumi"
	"deploy/pkg/cmd/remote"
)

// Global variable used to decide what main() should do.
var (
	Main = "pulumi"
)

// Note: this binary is both run by pulumi, and built to be ran on the
// vms that are built themselves.
//
// Basically, pulumi gets the default main.go, and from pulumi we
// rebuild the binary and set a linker arg to change out a global that
// controls which main we get.
//
// This way we can build the same main.go file and then copy it to the
// remotes for each group task to do stuff.
//
// Why do it like this? Simple, since we know we've the go compiler
// already, lets use it to build a binary to do the work on the vm's.
//
// This means we don't have to deal with ansible, chef, shell,
// whatever. The logic to "do stuff" is the same as the logic to build
// the stuff that does stuff. This makes the end user requirements
// that need to be installed to be simply: pulumi and the go compiler.
//
// Each vm "group" should be an argument. And each group should have a
// subgroup on action. This makes the pulumi side for any provider simple:
// - Pulumi gets vm's up and running up to the point they can be ssh'd into
// - Then we scp the remote binary to each node
// - Pulumi then runs that binary to "get crap done" for each group
// - We can then setup unit tests for all the logic local and remote
//
// Example: Lets say we have a group named k8s-server, and we simply
// have a create and destroy action for what to do to create a server
// or destroy it.
//
// cmd k8s-server create -> maps to execute k8s-server-create() on the vm
// Any args past that just map to anything a prime (first) node needs
// or to what a non prime or non first node needs.
//
// Example there might be the first node of a k8s server might need to
// be 'special' in some way. The rest might need that node to do
// 'something' before they can get their job done. Deciding when to
// run that is the pulumi side of things. Doing it is the binary and
// the group/action on the vm.
func main() {
	var err error
	if Main == "pulumi" {
		err = pulumi.Cmd()
	}
	if Main == "remote" {
		err = remote.Cmd()
	}
	if Main == "local" {
		err = local.Cmd()
	}

	if err != nil {
		log.Fatalf("%+v\n", err)
		os.Exit(1)
	}
}
