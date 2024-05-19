package ssh

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"deploy/pkg/cache"
	"deploy/pkg/ssh"
)

type Ssh struct {
	pulumi.ResourceState
	// Ssh key data
	Ssh *SshData
}

// This entire thing originated out of making it "easy" to have one
// type to control where .cache junk gets setup in and to have one
// spot to handle nuking/cleanup of it all. Also to store a bunch of
// data globally like where a dir prefix should be located.
//
// It keeps evolving more stuff as time goes on.
func NewSsh(ctx *pulumi.Context, state cache.State, name string, opts ...pulumi.ResourceOption) (*Ssh, error) {
	s := &Ssh{}

	prvKey := "ssh/id_rsa"
	pubKey := fmt.Sprintf("%s.pub", prvKey)

	prvKeyFile, err := state.RegisterArtifact(prvKey)
	if err != nil {
		return s, err
	}

	pubKeyFile, err := state.RegisterArtifact(pubKey)
	if err != nil {
		return s, err
	}

	sshKey, err := setupSshCa(ctx, pubKeyFile, prvKeyFile)
	if err != nil {
		return s, err
	}
	s.Ssh = &sshKey

	componentName := "shenanigans:ssh:key"

	err = ctx.RegisterComponentResource(componentName, name, s, opts...)
	if err != nil {
		return s, err
	}

	return s, nil
	return s, errors.New("break")
}

// TODOmoveme to ssh.go?
type SshData struct {
	PrvKey     *string
	PubKey     *string
	PrvKeyFile *string
	PubKeyFile *string
}

// Sets up the ssh ca resources and returns things a vm will need to setup/sign
// its host keys.
func setupSshCa(ctx *pulumi.Context, pub string, prv string) (SshData, error) {
	key, err := ssh.NewPrivateRsaKey(2048)
	if err != nil {
		return SshData{}, err
	}

	privateKeyData := ssh.EncodePrivateKeyToPEM(key)
	ctx.Export("caPrv", pulumi.String(prv))
	err = ssh.WriteSshkeyFile(privateKeyData, prv)
	if err != nil {
		return SshData{}, err
	}

	//	publicKeyFile := fmt.Sprintf("%s.pub", privateKeyFile)

	publicKeyData, err := ssh.NewPublicRsaKey(&key.PublicKey)
	if err != nil {
		return SshData{}, err
	}
	err = ssh.WriteSshkeyFile(publicKeyData, pub)
	if err != nil {
		return SshData{}, err
	}
	ctx.Export("caPub", pulumi.String(pub))

	prvd := string(privateKeyData)
	pubd := string(publicKeyData)

	// Trim the trailing newline
	prvd = strings.TrimSuffix(prvd, "\n")
	pubd = strings.TrimSuffix(pubd, "\n")

	return SshData{
		PrvKey:     &prvd,
		PubKey:     &pubd,
		PrvKeyFile: &prv,
		PubKeyFile: &pub,
	}, nil
}
