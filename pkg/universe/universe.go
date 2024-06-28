package universe

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"deploy/pkg/cache"
	"deploy/pkg/components/ssh"
	"deploy/pkg/filecache"
	"deploy/pkg/network"
	"deploy/pkg/unit"

	"deploy/pkg/providers/libvirt"
)

// Universe struct
// Gamma is our "universe" we're crafting, mostly a placeholder to
// encompass all the subcomponents.
//
// Mostly used to act as the backend to any transformations we might
// want to perform across groups.
//
// That is, if we have multiple "clusters", they likely will be
// sharing the same network, this lets us setup shared components
// between clusters in one spot that any group can access.
//
// This also is where any common setup occurs, e.g. compiling go
// binaries, setting up ssh keys etc...
//
// This is also whats doing the work to keep $XDG_HOME/.cache/NAME/ in
// order and its overall tidyness.
//
// TODOFuture work would be to start/stop http servers, pxe etc...
//
// Note this likely is doing "too goddamn much" but until I get a
// better handle on scope whatever. Each thing could become its own
// component ultimately.
type Universe struct {
	pulumi.ResourceState

	State *cache.State
	// TODOArray of Cached inputs in here too?
	Inputs *[]filecache.CachedFile

	Units *[]unit.Unit

	Network *network.Network

	// Ssh key data
	Ssh *ssh.SshData
}

// This entire thing originated out of making it "easy" to have one
// type to control where .cache junk gets setup in and to have one
// spot to handle nuking/cleanup of it all. Also to store a bunch of
// data globally like where a dir prefix should be located.
//
// It keeps evolving more stuff as time goes on.
//
// Not sure it makes sense to keep around in a way the "universe"
// should just be the top of the pulumi dag. This setup does make some
// stuff easier though so it'll stay until its a problem.
func NewUniverse(ctx *pulumi.Context, name string, opts ...pulumi.ResourceOption) (*Universe, error) {
	gamma := &Universe{}
	cfg := config.New(ctx, "shenanigans")

	// If we have an existing uuid for a prior build use that otherwise create one and export it
	cuuid, err := cfg.Try("uuid")

	gstate, err := cache.NewState("", cuuid)
	if err != nil {
		return gamma, err
	}
	gamma.State = gstate

	// Register the stack into outputs as well
	stack := ctx.Stack()
	ctx.Export("name", pulumi.String(stack))

	// Nuke the tmp dir on exit for partially downloaded cache files.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() error {
		<-sigs
		return filecache.CleanupCache()
	}()

	ctx.Export("base", pulumi.String(gamma.State.Base()))

	// Cover removal of the entire state dir with all the junk in
	// our trunk we create/generate at stack destroy time
	_, err = local.NewCommand(ctx, gamma.State.Uuid(),
		&local.CommandArgs{
			Delete: pulumi.Sprintf("rm -fr %s", gamma.State.Base()),
		},
	)

	var inputs []filecache.CachedFile
	cfg.RequireObject("inputs", &inputs)
	gamma.Inputs = &inputs

	// Convert user provided unit data to internal representation
	var userunits []unit.UserUnit
	cfg.RequireObject("units", &userunits)
	var units []unit.Unit

	for _, uu := range userunits {
		u, err := uu.ToUnit()
		if err != nil {
			return gamma, err
		}
		units = append(units, u)
		//		fmt.Printf("%s\n", u)
	}
	gamma.Units = &units

	var usernetwork network.UserNetwork
	cfg.RequireObject("network", &usernetwork)
	un, err := usernetwork.ToNetwork()
	if err != nil {
		return gamma, err
	}
	gamma.Network = &un

	// TODOEach cached file should really become its own component
	// that this depends upon Then I can let pulumi deal with
	// having N things run at once/download everything in parallel
	for _, can := range *gamma.Inputs {
		// fmt.Printf("dbg: %v\n", can)
		// fmt.Printf("dbg: %v\n", can.Sha256Sum)
		err := filecache.CacheInput(can)
		if err != nil {
			return gamma, err
		}
	}

	// For future we might have more than one ssh key. Why who the
	// hell knows. The name is more for the component atm tbh.
	sshKey, err := ssh.NewSsh(ctx, *gamma.State, "default")
	if err != nil {
		return gamma, err
	}
	gamma.Ssh = sshKey.Ssh

	remoteFile, err := gamma.State.RegisterArtifact("bin/remote")

	// Make this a component? Man I dunno anymore.
	cmd := exec.Command("go", "build", "-ldflags=-extldflags -static -s -w -X main.Main=remote", "-o", remoteFile)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "CGO_ENABLED=0")
	cmd.Env = append(cmd.Env, "GOARCH=amd64")
	cmd.Env = append(cmd.Env, "GOOS=linux")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
		fmt.Printf("out:\n%s\nerr:\n%s\ncmd.Run() failed with %s\n", outStr, errStr, err)
		return gamma, err
	}

	// _, err = local.NewCommand(ctx, "compile remote",
	// 	&local.CommandArgs{
	// 		Environment: pulumi.StringMap{
	// 			"CGO_ENABLED": pulumi.String("0"),
	// 			"GOARCH":      pulumi.String("amd64"),
	// 			"GOOS":        pulumi.String("linux"),
	// 		},
	// 		Create: pulumi.Sprintf("go build -ldflags='-extldflags -static -s -w -X main.Main=remote' -o %s", remoteFile),
	// 		//			Delete: pulumi.Sprintf("rm -fr %s", *gamma.prefix), // Only need to remove the prefix dir data
	// 	},
	// )

	// Setup shared resources for this universe.  e.g. for libvirt
	// things like network and pool location that everything else
	// will use.

	// I'm a hack and hate object oriented, just make this a loop
	// through providers at some point.
	shared, err := libvirt.SetupShared(ctx, gamma.State, gamma.Network)

	if err != nil {
		return gamma, err
		fmt.Printf("shared: %v", shared)
	}

	// Golang is the lamest excuse for a language ever, what can
	// be in a map is stupid so work around it by using the
	// Unit.Name and using that as the "key".
	hackMap := make(map[string]unit.Unit)

	// setup the unitMap/hackmap
	for _, u := range *gamma.Units {
		_, e := hackMap[u.Name]
		if !e {
			hackMap[u.Name] = u
		}

		// For any unit with no dependencies add it to the dag now
		//
		// If stuff depends on it we'll use that in the next loop.
		if len(u.After) == 0 {
			err = libvirt.Unit(ctx, gamma.State, &u, &shared, gamma.Ssh, gamma.Inputs, gamma.Network)
			if err != nil {
				return gamma, err
			}
		}
	}

	componentName := "shenanigans:universe:Universe"

	// Note we pass in gamma which should be the goal of the
	// entire universe. Maybe in the future I make a way to
	// connect universes but for now there is only one like the
	// one we live in. (It is the authors opinion that multi
	// worlds is a philisophical hack around quantum physics lack
	// of progression beyond the copenhagen model for the past
	// 80ish years.)
	err = ctx.RegisterComponentResource(componentName, stack, gamma, opts...)
	if err != nil {
		return gamma, err
	}

	return gamma, nil
	return gamma, errors.New("break")
}
