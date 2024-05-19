package gamma

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path"
	//	"strings"
	"syscall"

	"github.com/adrg/xdg"
	"github.com/google/uuid"
	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"deploy/pkg/filecache"
	"deploy/pkg/universe"
	//	"deploy/pkg/group"
	//	"deploy/pkg/ssh"
	"deploy/pkg/unit"

	"deploy/pkg/providers/libvirt"
)

// This entire thing originated out of making it "easy" to have one
// type to control where .cache junk gets setup in and to have one
// spot to handle nuking/cleanup of it all. Also to store a bunch of
// data globally like where a dir prefix should be located.
//
// It keeps evolving more stuff as time goes on.
func NewGamma(ctx *pulumi.Context, name string, opts ...pulumi.ResourceOption) (*universe.Universe, error) {
	gamma := &universe.Universe{}
	cfg := config.New(ctx, "shenanigans")

	// If we have an existing uuid for a prior build use that otherwise create one and export it
	cuuid, err := cfg.Try("uuid")
	if err != nil {
		nuuid := uuid.New().String()
		gamma.Uuid = &nuuid
		ctx.Export("uuid", pulumi.String(*gamma.Uuid))
	} else {
		gamma.Uuid = &cuuid
	}

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

	// Setup .cache dir
	baseDir := path.Join(xdg.CacheHome, "shenanigans")
	prefix := path.Join(baseDir, "instances", *gamma.Uuid)
	ctx.Export("prefix", pulumi.String(prefix))

	err = os.MkdirAll(prefix, os.ModePerm)
	if err != nil {
		return gamma, err
	}
	gamma.Prefix = &prefix

	// Cover removal of the entire instance dir with all the junk
	// in our trunk we create/generate
	_, err = local.NewCommand(ctx, *gamma.Uuid,
		&local.CommandArgs{
			//			Create: pulumi.Sprintf("install -dm755 %s", path.Join(dir, "artifacts", stack)),
			Delete: pulumi.Sprintf("rm -fr %s", *gamma.Prefix), // Only need to remove the prefix dir data
		},
	)

	tmp := path.Join(prefix, "tmp")
	err = os.MkdirAll(tmp, os.ModePerm)
	if err != nil {
		return gamma, err
	}
	gamma.Tmp = &tmp

	artifacts := path.Join(prefix, "artifacts")
	ctx.Export("artifacts", pulumi.String(artifacts))
	err = os.MkdirAll(artifacts, os.ModePerm)
	if err != nil {
		return gamma, err
	}
	gamma.Artifacts = &artifacts

	instance := path.Join(prefix, "instance")
	err = os.MkdirAll(instance, os.ModePerm)
	if err != nil {
		return gamma, err
	}
	gamma.Instance = &instance

	cache := path.Join(baseDir, "cache")
	err = os.MkdirAll(cache, os.ModePerm)
	if err != nil {
		return gamma, err
	}
	gamma.Cache = &cache

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
		fmt.Printf("%s\n", u)
	}
	gamma.Units = &units

	// TODOEach cached file should really become its own component
	// that this depends upon Then I can let pulumi deal with
	// having N things run at once/download everything in parallel
	for _, can := range *gamma.Inputs {
		err := filecache.CacheInput(can)
		if err != nil {
			return gamma, err
		}
	}

	sshDir := path.Join(artifacts, "ssh")
	ctx.Export("sshDir", pulumi.String(sshDir))

	err = os.MkdirAll(sshDir, os.ModePerm)
	if err != nil {
		return gamma, err
	}

	sshKey, err := universe.SetupSshCa(ctx, *gamma.Uuid, artifacts)
	if err != nil {
		return gamma, err
	}
	gamma.Ssh = &sshKey

	remoteFile := path.Join(artifacts, "go", "remote")

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
		fmt.Printf("out:\n%s\nerr:\n%s\n", outStr, errStr)
		fmt.Printf("cmd.Run() failed with %s\n", err)
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

	gamma.Remote = &remoteFile

	ctx.Export("remote", pulumi.String(remoteFile))

	// Setup shared resources for this universe.  e.g. for libvirt
	// things like network and pool location that everything else
	// will use.

	// err = libvirt.SetupShared(ctx, gamma)
	// if err != nil {
	// 	return gamma, err
	// }

	componentName := "shenanigans:universe:Universe"

	// Note we pass in gamma which should be the goal of the
	// entire universe. Maybe in the future I make a way to
	// connect universes but for now there is only one like the
	// one we live in. (It is the authors opinion that multi
	// worlds is a philisophical hack around quantum physics lack
	// of progression beyond the copenhagen model for the past
	// 80ish years.)
	err = ctx.RegisterComponentResource(componentName, name, gamma, opts...)
	if err != nil {
		return gamma, err
	}

	return gamma, nil
	return gamma, errors.New("break")
}
