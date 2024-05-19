package groups

import (
	"bytes"
	"errors"
	"fmt"
	//	"io/ioutil"
	"os"
	//	"os/signal"
	"path"
	//	"path/filepath"
	//	"syscall"
	"text/template"

	// for later...
	// cloudinit "github.com/pulumi/pulumi-cloudinit/sdk"

	//	"github.com/adrg/xdg"
	"github.com/dustinkirkland/golang-petname"
	"github.com/google/uuid"
	//	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	//	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi-libvirt/sdk/go/libvirt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	//	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	//	"github.com/juju/juju/cloudconfig/cloudinit"

	"deploy/pkg/components/gamma"
	//	"deploy/pkg/filecache"
	"deploy/pkg/group"
)

const GB = 1024 * 1024 * 1024

func ParseTemplate(fileName string, data any) (string, error) {
	f, err := os.ReadFile(fileName)
	if err != nil {
		return "", err
	}
	templ := template.Must(template.New("template").Parse(string(f)))
	var buf bytes.Buffer
	err = templ.Execute(&buf, data)
	return buf.String(), err
}

// Generic network configuration, for now just the network cidr
//
// For now ipv4 only is assumed.
type NetworkConfig struct {
	Cidr string
	//TODOstatic hostname array to let groups specify they want some static ip's setup in the resulting network? Future mitch figure it out this is a past mitch jerk move
}

// For vm "groups" this is our backing type that records the name of
// the vm, and its ip address (should be routeable).
type VMGroups struct {
	pulumi.ResourceState

	Vms map[string][]VirtualMachine `pulumi:"vms"`
}

type VirtualMachine struct {
	Name pulumi.StringOutput `pulumi:"name"`
	Ipv4 pulumi.StringOutput `pulumi:"ipv4"`
	// ipv6 later
}

type LibvirtConfig struct {
	Uri     string
	Pooldir string
}

// Internal type for sharing data, least the start of it for
// global/shared setup in libvirt.
type libVirtConfig struct {
	Provider             libvirt.Provider
	Network              libvirt.Network
	CloudInitStoragePool libvirt.Pool
	VmStoragePool        libvirt.Pool
	BaseVolume           libvirt.Volume // TODOmultiple base volumes means move outta hear
}

type User struct {
	Name              string
	SSHAuthorizedKeys []string
	Password          string
}

type UserData struct {
	Users      []User
	Hostname   string
	Sshkey     string
	RemoteData string
}

// Wrapper function that creates all groups and sets up dependencies
// as needed, bit of a god object/function for now.
//
// This function is also what sets up things that might be in common
// to groups e.g. for libvirt a common network setup between groups,
// the provider etc...
//
// Until I stop being lazy and handle data sharing between things
// sanely hard coding group dependency data cause whatever.
//
// Also most of this has zero dynamicism between backend providers.
func CreateConfig(ctx *pulumi.Context, gamma *gamma.Gamma) error {
	var providerconfig LibvirtConfig
	var libvirtconfig libVirtConfig

	//	return errors.New(fmt.Sprintf("groups: %v\n", groups))

	//	cfg.RequireObject("libvirt", &providerconfig)

	//	if providerconfig.Uri == "" {
	providerconfig.Uri = "qemu+unix:///system"
	//	}

	// TODOIf I let people specify this in the config how do I make sure its sane?
	//	if providerconfig.Pooldir == "" {
	providerconfig.Pooldir = "/var/lib/libvirt/shenanigans"
	//	}

	provider, err := libvirt.NewProvider(ctx, "provider", &libvirt.ProviderArgs{
		Uri: pulumi.String(providerconfig.Uri),
	})
	if err != nil {
		return err
	}

	libvirtconfig.Provider = *provider

	ctx.Export("provider", pulumi.String("libvirt"))
	ctx.Export("libvirt-uri", pulumi.String(providerconfig.Uri))
	ctx.Export("libvirt-pooldir", pulumi.String(providerconfig.Pooldir))

	poolDir := path.Join(providerconfig.Pooldir, *gamma.Uuid)

	network, err := libvirt.NewNetwork(ctx, *gamma.Uuid, &libvirt.NetworkArgs{
		// TODO: address range selection for ipv4/6? future me problem
		Name:      pulumi.String(*gamma.Uuid),
		Addresses: pulumi.StringArray{pulumi.String("10.200.200.1/24")}, // TODOmake this a network option and be dynamic based off the stack name? Or maybe random subnets.
		Autostart: pulumi.Bool(true),
		Mode:      pulumi.String("nat"),
		// Mode:   pulumi.String("bridge"),
		// Bridge: pulumi.String("virbr1"),
		Domain: pulumi.String("dev.home.arpa"),
		Dhcp: &libvirt.NetworkDhcpArgs{
			Enabled: pulumi.Bool(true),
		},
		Dns: &libvirt.NetworkDnsArgs{
			Enabled: pulumi.Bool(true),
			//			LocalOnly: pulumi.Bool(false),
		},
	}) //, pulumi.Parent(&resource), pulumi.DeleteBeforeReplace(true))
	if err != nil {
		return err
	}
	libvirtconfig.Network = *network

	cipool, err := libvirt.NewPool(ctx, fmt.Sprintf("%s-cloudinit", *gamma.Uuid), &libvirt.PoolArgs{
		Type: pulumi.String("dir"),
		Path: pulumi.String(path.Join(poolDir, "cloudinit"))})
	if err != nil {
		return err
	}
	libvirtconfig.CloudInitStoragePool = *cipool

	vmpool, err := libvirt.NewPool(ctx, fmt.Sprintf("%s-vm", *gamma.Uuid), &libvirt.PoolArgs{
		Type: pulumi.String("dir"),
		Path: pulumi.String(path.Join(poolDir, "vm"))})
	if err != nil {
		return err
	}
	libvirtconfig.VmStoragePool = *vmpool

	// file := "989f3aa9a1ef5a3e289620c7190c62831a6f1b9669506edab6f713a956e7a976"
	// baseName := "basevol-test"
	// //	return errors.New(fmt.Sprintf("%s\n", path.Join(*gamma.Cache, file)))
	// baseVolName := pulumi.String("testvolume")
	// _, err = libvirt.NewVolume(ctx, baseName, &libvirt.VolumeArgs{
	// 	Name:   baseVolName,
	// 	Pool:   libvirtconfig.VmStoragePool.Name,
	// 	Source: pulumi.Sprintf("file://%s", path.Join(*gamma.Cache, file)),
	// }, pulumi.Provider(provider), pulumi.DeleteBeforeReplace(true),
	// )

	// if err != nil {
	// 	return err
	// }

	for _, onegroup := range *gamma.Groups {
		if len(onegroup.Config) > 0 {
			fmt.Printf("creating group %v\n", onegroup)
			err := createGroup(ctx, gamma, onegroup, provider, &libvirtconfig)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type CloudInitUserData struct {
	finalMessage string              `yaml:"final_message"`
	output       string              `yaml:"output"`
	hostname     string              `yaml:"hostname"`
	fqdn         string              `yaml:"fqdn"`
	sshPwauth    bool                `yaml:"ssh_pwauth"`
	chpasswd     CloudInitChPasswd   `yaml:"chpasswd"`
	writeFiles   []CloudInitFileData `yaml:"write_files"`
}

type CloudInitFileData struct {
	path        string `yaml:"path"`
	owner       string `yaml:"owner"`
	permissions string `yaml:"permissions"`
	content     string `yaml:"content"`
}

type CloudInitChPasswd struct {
	expire bool                  `yaml:"expire"`
	users  []CloudInitUserConfig `yaml:"users"`
}

type CloudInitUserConfig struct {
	name     string `yaml:"name"`
	password string `yaml:"password"`
	usertype string `yaml:"type"` // TODOshould be an enum at some point
}

// Internal function that creates a group on a provider (generally)
//
// Bit of a god function atm, future me can make it better once I start adding more providers when it'll matter more.
func createGroup(ctx *pulumi.Context, gamma *gamma.Gamma, onegroup group.Group, provider *libvirt.Provider, libvirtconf *libVirtConfig, opts ...pulumi.ResourceOption) error {
	// Create a hard link to:
	// ~/.cache/shenanigans/cache/SHA256SUM
	// from:
	// ~/.cache/shenanigans/instances/INSTANCEUUID.qcow2

	// Note that the cachefile is done entirely outside of pulumi
	// config so it can be used across all instances that might
	// get built between stacks. Saves on downloading crap constantly.
	var vmDomains []libvirt.Domain
	//	var vmDepends []pulumi.Resource

	// Note we can have multiple configurations in a group, the
	// kind determines what matters/when between groups.

	// For now the only special group is k8s which uses the
	// default config for control planes and on top the first node
	// is used as the first control plane node for other k8s nodes
	// to join/register against.
	//
	// If/when more junk needs building this all should likely be
	// genericized somehow. Can't brain a good way for now so left
	// to future mitch as is tradition. TODO
	//
	// Note all vm's in a group are brought online at once, hidden
	// in this is future non k8s kind groups of vm's which should
	// likely be a function.

	//	var defaultPrime pulumi.Resource

	for _, groupConfig := range onegroup.Config {
		//	if onegroup.Name == "default" {}

		// Each volume gets its own symlink to its backing
		// image file. This exists for libvirt only to put a
		// .qcow2 suffix to the filename or libvirt goes nuts
		// saying amgs wat is this blob of bits I couldn't
		// possibly look at the file to see if its a qcow2 or
		// anything I'll just throw an error. I'm not bitter
		// or anything really.
		baseVolUuid := uuid.New().String()

		// TODO This is a hack I should just pass in the local
		// filename directly
		cacheFile := path.Join(*gamma.Cache, groupConfig.Qcow2)
		instFile := path.Join(*gamma.Instance, fmt.Sprintf("%s.qcow2", baseVolUuid))
		err := os.Link(cacheFile, instFile)

		if err != nil {
			return err
		}

		// Create a volume off the base qcow to use
		baseVolName := pulumi.Sprintf(baseVolUuid)
		baseVolume, err := libvirt.NewVolume(ctx, baseVolUuid, &libvirt.VolumeArgs{
			Name:   baseVolName,
			Pool:   libvirtconf.VmStoragePool.Name,
			Source: pulumi.Sprintf("file://%s", cacheFile),
		}, pulumi.Provider(provider),
		)

		if err != nil {
			return err
		}

		// return errors.New(fmt.Sprintf("vm config is %v\n", groupConfig))
		// TODO vm setup here

		// Record idx 0 (prime) vm of the default group, its
		// "special" in that it gets abused for a lot of logic
		// as its the only guaranteed node to exist.
		for idx := 0; idx < groupConfig.Count; idx++ {
			// if (idx == 0) && (groupConfig.Name == "default") {
			// 	prime := true
			// } else {
			// 	prime := false
			// }

			var userdata UserData

			// TODOmaybe detect this somehow in cloud-init setup?
			//vmIface := "eth0"

			hostName := petname.Generate(3, "-")

			userdata.Hostname = hostName
			userdata.Sshkey = *gamma.Ssh.PubKey

			vmInstUuid := uuid.New().String()
			fileSystem, err := libvirt.NewVolume(ctx, fmt.Sprintf("%s-disk%d", vmInstUuid, 1), &libvirt.VolumeArgs{
				Pool:           libvirtconf.VmStoragePool.Name,
				BaseVolumeName: baseVolume.Name,
				BaseVolumePool: libvirtconf.VmStoragePool.Name,
				Size:           pulumi.Int(GB * groupConfig.Disksize),
			}, pulumi.Provider(provider),
			)

			if err != nil {
				return err
			}

			// ciArtDir := path.Join(*gamma.Artifacts, "cloudinit", hostName)
			// os.MkdirAll(ciArtDir, 0755)

			// cloudInitUserData, err := ParseTemplate("./cloud_init_user_data.yaml.gotmpl", userdata)
			// if err != nil {
			// 	return err
			// }

			// cloudInitMetaData, err := ParseTemplate("./cloud_init_meta_data.yaml.gotmpl", vm)
			// if err != nil {
			// 	return err
			// }

			// userData := path.Join(ciArtDir, "user_data")
			// err = ioutil.WriteFile(userData, []byte(cloudInitUserData), 0600)
			// if err != nil {
			// 	return err
			// }
			// metaData := path.Join(ciArtDir, "meta_data")
			// err = ioutil.WriteFile(metaData, []byte(cloudInitMetaData), 0600)
			// if err != nil {
			// 	return err
			// }

			// Avoid templates, just build up a cloud init
			// config yaml from scratch using plain old
			// code. No need to involve layers of stupid
			// to write out yaml in the end.

			// We'll just define our own structs and do it
			// the hard way. There is little need for a
			// template to be honest, just edit the source
			// code why are we bothering with another
			// layer of indirection.
			// cloudInit, err := libvirt.NewCloudInitDisk(ctx, fmt.Sprintf("%s-%d-cloudinit", *gamma.Uuid, idx), &libvirt.CloudInitDiskArgs{
			// 	MetaData:      pulumi.String(string(cloudInitMetaData)),
			// 	NetworkConfig: pulumi.String(string(cloudInitMetaData)),
			// 	UserData:      pulumi.String(string(cloudInitUserData)),
			// 	Pool:          cloudInitStoragePool.Name,
			// }, pulumi.Provider(provider))
			// if err != nil {
			// 	return err
			// }

			lvDomain, err := libvirt.NewDomain(ctx, hostName, &libvirt.DomainArgs{
				Memory: pulumi.Int(groupConfig.Memory * 1024),
				Vcpu:   pulumi.Int(groupConfig.Cpu),
				//				Cloudinit: cloudInit.ID(),
				Disks: libvirt.DomainDiskArray{
					libvirt.DomainDiskArgs{
						VolumeId: fileSystem.ID(),
					},
				},
				NetworkInterfaces: libvirt.DomainNetworkInterfaceArray{
					libvirt.DomainNetworkInterfaceArgs{
						NetworkName: libvirtconf.Network.Name,
						Hostname:    pulumi.String(hostName),
						//TODO afer cloudinit is working uncomment
						//						WaitForLease: pulumi.Bool(true), // Need ip's bra
					},
				},
				Consoles: libvirt.DomainConsoleArray{
					libvirt.DomainConsoleArgs{
						Type:       pulumi.String("pty"),
						TargetPort: pulumi.String("0"),
						TargetType: pulumi.String("serial"),
					},
				},
			},
				pulumi.Provider(provider),
				pulumi.ReplaceOnChanges([]string{"*"}),
				pulumi.DeleteBeforeReplace(true),
			)

			if err != nil {
				return err
			}

			// 	resource.IP = domain.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))

			// pwConnectionArgs := remote.ConnectionArgs{
			// 	Host:           lvDomain.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0)),
			// 	User:           pulumi.String("root"),
			// 	Password:       pulumi.String("changeme"),
			// 	PerDialTimeout: pulumi.Int(3),
			// 	DialErrorLimit: pulumi.Int(80),
			// 	// PrivateKey: pulumi.Sprintf("%s", sshKey.PrvKeyFile),
			// }

			// keyConnectionArgs := remote.ConnectionArgs{
			// 	Host:       lvDomain.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0)),
			// 	User:       pulumi.String("root"),
			// 	PrivateKey: pulumi.Sprintf("%s", *gamma.Ssh.PrvKey),
			// }

			// // Note for some os's until you login with a password keys don't work. TODO is figure out why.
			// ensureSshKey, err := remote.NewCommand(ctx, fmt.Sprintf("%s ssh pass?", vmName), &remote.CommandArgs{
			// 	Connection: keyConnectionArgs,
			// 	Create:     pulumi.String("uptime"),
			// }, pulumi.DependsOn([]pulumi.Resource{lvDomain}),
			// )
			// if err != nil {
			// 	return err
			// }

			// remoteRemoteFile := "/usr/local/sbin/remote"
			// // Future mitch, don't put this crap into cloud-init
			// // again, that like 10x's the cloud-init run time.
			// //
			// // This is faster by far, if janky af.
			// copyRemote, err := remote.NewCopyFile(ctx, fmt.Sprintf("vm%d copy remote", idx), &remote.CopyFileArgs{
			// 	Connection: keyConnectionArgs,
			// 	LocalPath:  pulumi.String(*gamma.Remote),
			// 	RemotePath: pulumi.String(remoteRemoteFile),
			// }, pulumi.DependsOn([]pulumi.Resource{ensureSshKey}))
			// if err != nil {
			// 	return err
			// }

			// // Note for some os's until you login with a password keys don't work. TODO is figure out why.
			// remoteOk, err := remote.NewCommand(ctx, fmt.Sprintf("%s remote ok?", vmName), &remote.CommandArgs{
			// 	Connection: keyConnectionArgs,
			// 	Create:     pulumi.Sprintf("chmod 755 %s && %s initcheck", remoteRemoteFile, remoteRemoteFile),
			// }, pulumi.DependsOn([]pulumi.Resource{ensureSshKey, copyRemote}),
			// )
			// if err != nil {
			// 	return err
			// }
			// vmDepends = append(vmDepends, remoteOk)

			// vmSetup, err := remote.NewCommand(ctx, fmt.Sprintf("%s motd?", vmName), &remote.CommandArgs{
			// 	Connection: keyConnectionArgs,
			// 	Create:     pulumi.Sprintf("install -m644 /dev/null /etc/motd"),
			// }, pulumi.DependsOn([]pulumi.Resource{ensureSshKey}),
			// )
			// if err != nil {
			// 	return err
			// }

			vmDomains = append(vmDomains, *lvDomain)
			//			vmDepends = append(vmDepends, vmSetup)
		}
	}

	for idx, groupConfig := range onegroup.Config {
		fmt.Printf("register %d\n", idx)
		// Register each group/config as a separate component

		name := fmt.Sprintf("%s:%s", onegroup.Name, groupConfig.Name)

		componentName := "shenanigans:group"

		// componentExists, err := resource.GetResource(ctx, name, componentName, "resourceId")
		// if err != nil {
		// 	return err
		// }

		err := ctx.RegisterComponentResource(componentName, name, gamma, opts...)
		if err != nil {
			fmt.Printf("%v\n", err)
			return err
		}
	}

	// if kind == k8s setup here should be a function at some point
	//
	// Need to figure out how I want to make dependencies between
	// providers and groups of things dynamic
	// TODOFuture mitch brain up a way to do all this crap sucks to be you!

	if false {
		return errors.New("zomg")
	}

	return nil
	// //	var sshConfigRequired []pulumi.Resource
	// var sshConfigFileContent pulumi.StringOutput
	// sshConfigFileContent = pulumi.Sprintf("%s", "")

	// for idx, vm := range vmDomains {
	// 	ip4 := vm.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
	// 	// host := vm.NetworkInterfaces.Index(pulumi.Int(0)).Hostname()
	// 	host := vm.Name
	// 	//		uuid := vm.Name

	// 	add := pulumi.All(vm, host, idx, ip4).ApplyT(func(args []interface{}) string {
	// 		return fmt.Sprintf("Host %s vm%d\n  Hostname %s\n  IdentityFile %s\n  User %s\n  UserKnownHostsFile /dev/null\n  StrictHostKeyChecking no\n  LogLevel QUIET\n", args[1], args[2], args[3], *gamma.Ssh.PrvKeyFile, "root")
	// 	})
	// 	sshConfigFileContent = pulumi.Sprintf("%s%s", sshConfigFileContent, add)
	// 	//		sshConfigRequired = append(sshConfigRequired, vm)
	// }

	// sshConfigFile := path.Join(*gamma.Artifacts, "ssh", "config")
	// sshConfigFileWriter, err := local.NewCommand(ctx,
	// 	"ssh-config",
	// 	&local.CommandArgs{
	// 		Create: pulumi.Sprintf("echo '%s' > %s", sshConfigFileContent, sshConfigFile),
	// 		Delete: pulumi.Sprintf("rm -f %s", sshConfigFile),
	// 	},
	// 	//		pulumi.DependsOn(sshConfigRequired),
	// )

	// ctx.Export("sshconfig", pulumi.String(sshConfigFile))

	// ciLogName := "cloud-init-output.log"
	// ciRemoteLogFile := path.Join("/var/log", ciLogName)

	// for idx, _ := range vmDomains {
	// 	vmHost := fmt.Sprintf("vm%d", idx)
	// 	ciArtDir := path.Join(*gamma.Artifacts, "cloudinit", vmHost)
	// 	os.MkdirAll(ciArtDir, 0755)

	// 	ciLogFile := path.Join(ciArtDir, ciLogName)

	// 	ciLogDepends := vmDepends
	// 	ciLogDepends = append(ciLogDepends, sshConfigFileWriter)

	// 	_, err = local.NewCommand(ctx,
	// 		fmt.Sprintf("vm%d copy cloud-init log", idx),
	// 		&local.CommandArgs{
	// 			Create: pulumi.Sprintf("ssh -F %s %s 'cat %s' > %s", sshConfigFile, vmHost, ciRemoteLogFile, ciLogFile),
	// 		},
	// 		pulumi.DependsOn(ciLogDepends))

	// 	if err != nil {
	// 		return err
	// 	}
	// }

	// // // TODO move this crap into some sorta higher level grouping of shit
	// // for idx, vm := range vmDomains {
	// // 	ip4 := vm.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
	// // 	host := vm.Name

	// // 	// Abusing the instance uuid to act as the rke2 join
	// // 	// token, sue me
	// // 	rke2token := *gamma.Uuid

	// // 	// Copy in the cached rke2 artifacts that the remote
	// // 	// binary needs to the dirs they are expected to be in
	// // 	add := pulumi.All(vm, host, idx, ip4).ApplyT(func(args []interface{}) string {
	// // 		return fmt.Sprintf("Host %s vm%d\n  Hostname %s\n  IdentityFile %s\n  User %s\n  UserKnownHostsFile /dev/null\n  StrictHostKeyChecking no\n  LogLevel QUIET\n", args[1], args[2], args[3], *gamma.Ssh.PrvKeyFile, "root")
	// // 	})
	// // 	sshConfigFileContent = pulumi.Sprintf("%s%s", sshConfigFileContent, add)
	// // 	//		sshConfigRequired = append(sshConfigRequired, vm)
	// // }

	// var inputs []cache.CacheFile
	// cfg.RequireObject("inputs", &inputs)

	// for idx, vm := range vmDomains {
	// 	ip4 := vm.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
	// 	keyConnectionArgs := remote.ConnectionArgs{
	// 		Host:       ip4,
	// 		User:       pulumi.String("root"),
	// 		PrivateKey: pulumi.Sprintf("%s", *gamma.Ssh.PrvKey),
	// 	}

	// 	//		vmHost := fmt.Sprintf("vm%d", idx)
	// 	for _, f := range inputs {
	// 		cacheFile := path.Join(cachePrefix, f.Sha256Sum)
	// 		remoteFile := f.Remote.Dest
	// 		remoteFileTmp := fmt.Sprintf("%s.tmp", f.Remote.Dest)

	// 		base := filepath.Dir(remoteFile)
	// 		if remoteFile != "" {
	// 			pulumi.Sprintf("vm%d input: %s -> %s\n", idx, cacheFile, remoteFile)

	// 			baseDir, err := remote.NewCommand(ctx,
	// 				fmt.Sprintf("vm%d basedir %s", idx, remoteFile),
	// 				&remote.CommandArgs{
	// 					Connection: keyConnectionArgs,
	// 					Create:     pulumi.Sprintf("mkdir -p %s || :", base),
	// 				}, pulumi.DependsOn([]pulumi.Resource{sshConfigFileWriter}),
	// 			)

	// 			if err != nil {
	// 				return err
	// 			}

	// 			copyInput, err := remote.NewCopyFile(ctx, fmt.Sprintf("vm%d copy %s", idx, remoteFile), &remote.CopyFileArgs{
	// 				Connection: keyConnectionArgs,
	// 				LocalPath:  pulumi.String(cacheFile),
	// 				RemotePath: pulumi.String(remoteFileTmp),
	// 			}, pulumi.DependsOn([]pulumi.Resource{baseDir}))
	// 			if err != nil {
	// 				return err
	// 			}

	// 			installFile, err := remote.NewCommand(ctx,
	// 				fmt.Sprintf("vm%d copy %s", idx, remoteFile),
	// 				&remote.CommandArgs{
	// 					Connection: keyConnectionArgs,
	// 					Create:     pulumi.Sprintf("install -m%s --owner %s --group %s %s %s && rm %s", f.Remote.Mode, f.Remote.Owner, f.Remote.Group, remoteFileTmp, remoteFile, remoteFileTmp),
	// 				}, pulumi.DependsOn([]pulumi.Resource{copyInput}))

	// 			if err != nil {
	// 				return err
	// 			}
	// 			vmDepends = append(vmDepends, installFile)
	// 		}
	// 	}
	// }

	// primeip4 := pulumi.StringOutput{}

	// var k8sDepends []pulumi.Resource

	// for idx, vm := range vmDomains {
	// 	ip4 := vm.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
	// 	keyConnectionArgs := remote.ConnectionArgs{
	// 		Host:       ip4,
	// 		User:       pulumi.String("root"),
	// 		PrivateKey: pulumi.Sprintf("%s", *gamma.Ssh.PrvKey),
	// 	}

	// 	vmHost := fmt.Sprintf("vm%d", idx)

	// 	if idx == 0 {
	// 		primeip4 = ip4
	// 		ctx.Export("primeip4", primeip4)
	// 		k8sInstall, err := remote.NewCommand(ctx,
	// 			fmt.Sprintf("vm%d k8s prime", idx),
	// 			&remote.CommandArgs{
	// 				Connection: keyConnectionArgs,
	// 				Create:     pulumi.Sprintf("%s k8s --prime", "/usr/local/sbin/remote"),
	// 			}, pulumi.DependsOn(vmDepends),
	// 		)

	// 		if err != nil {
	// 			return err
	// 		}

	// 		k8sDepends = append(k8sDepends, k8sInstall)
	// 		remoteKubeconfig := "/etc/rancher/rke2/rke2.yaml"

	// 		k8sStackDir := path.Join(*gamma.Artifacts, "k8s", vmHost)
	// 		localKubeconfig := path.Join(k8sStackDir, "kubeconfig")

	// 		os.MkdirAll(k8sStackDir, 0755)

	// 		_, err = local.NewCommand(ctx,
	// 			fmt.Sprintf("vm%d copy kubeconfig", idx),
	// 			&local.CommandArgs{
	// 				Create: pulumi.Sprintf("ssh -F %s %s 'cat %s | sed -e \"s/127.0.0.1/%s/\"' > %s", sshConfigFile, vmHost, remoteKubeconfig, ip4, localKubeconfig),
	// 			},
	// 			pulumi.DependsOn([]pulumi.Resource{sshConfigFileWriter, k8sInstall}))

	// 		if err != nil {
	// 			return err
	// 		}
	// 	} else {
	// 		_, err = remote.NewCommand(ctx,
	// 			fmt.Sprintf("vm%d k8s worker", idx),
	// 			&remote.CommandArgs{
	// 				Connection: keyConnectionArgs,
	// 				Create:     pulumi.Sprintf("%s k8s --worker --upstream %s", "/usr/local/sbin/remote", primeip4),
	// 			}, pulumi.DependsOn(k8sDepends),
	// 		)

	// 		if err != nil {
	// 			return err
	// 		}
	// 	}
	// }
	// return nil
}
