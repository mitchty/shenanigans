package libvirt

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path"
	"path/filepath"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	//"github.com/pulumi/pulumi-kubernetes-cert-manager/sdk/go/kubernetes-cert-manager"
	// "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	// "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-libvirt/sdk/go/libvirt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/dustinkirkland/golang-petname"

	"deploy/pkg/cache"
	"deploy/pkg/cloudinit"
	"deploy/pkg/components/ssh"
	"deploy/pkg/filecache"
	"deploy/pkg/unit"
	"deploy/pkg/vm"
)

// Generic network configuration, for now just the network cidr
//
// For now ipv4 only is assumed.
type NetworkConfig struct {
	Cidr string
	//TODO static hostname array to let groups specify they want some static ip's setup in the resulting network? Future mitch figure it out this is a past mitch jerk move
}

// For vm "groups" this is our backing type that records the name of
// the vm, and its ip address (should be routeable).
type VMGroups struct {
	pulumi.ResourceState

	Vms map[string][]VirtualMachine `pulumi:"vms"`
}

type VirtualMachine struct {
	pulumi.ResourceState
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
type LibvirtShared struct {
	Provider             libvirt.Provider
	Network              libvirt.Network
	CloudInitStoragePool libvirt.Pool
	VmStoragePool        libvirt.Pool
	BaseVolume           libvirt.Volume // TODO multiple base volumes means move outta hear
}

// TODO This needs to get broken out of this module
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

// I'm getting sick of "duplicate" urn's that aren't duplicates in pulumi
// TODO move me somewhere better
const randRunes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(length int) (string, error) {
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(randRunes))))
		if err != nil {
			return "", err
		}
		b[i] = randRunes[n.Int64()]
	}
	return string(b), nil
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
func SetupShared(ctx *pulumi.Context, state *cache.State) (LibvirtShared, error) {
	var providerconfig LibvirtConfig
	var shared LibvirtShared

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
		return shared, err
	}

	shared.Provider = *provider

	ctx.Export("provider", pulumi.String("libvirt"))
	ctx.Export("libvirt-uri", pulumi.String(providerconfig.Uri))
	ctx.Export("libvirt-pooldir", pulumi.String(providerconfig.Pooldir))

	poolDir := path.Join(providerconfig.Pooldir, state.Uuid())

	network, err := libvirt.NewNetwork(ctx, state.Uuid(), &libvirt.NetworkArgs{
		// TODO: address range selection for ipv4/6? future me problem
		Name: pulumi.String(state.Uuid()),
		Addresses: pulumi.StringArray{
			pulumi.String("10.200.200.1/24"),
			//			pulumi.String("2001:db8:ca2:2::1/64"),
		}, // TODOmake this a network option and be dynamic based off the stack name? Or maybe random subnets.
		Autostart: pulumi.Bool(true),
		Mode:      pulumi.String("nat"),

		// Mode:   pulumi.String("bridge"),
		// Bridge: pulumi.String("virbr1"),
		Domain: pulumi.String("dev.home.arpa"),
		Dhcp: &libvirt.NetworkDhcpArgs{
			Enabled: pulumi.Bool(true),
		},
		Dns: &libvirt.NetworkDnsArgs{
			Enabled:   pulumi.Bool(true),
			LocalOnly: pulumi.Bool(true), // Don't forward local requests otherwise we end up looping if this dnsmasq instance is queried from the outside.
			Hosts: libvirt.NetworkDnsHostArray{
				&libvirt.NetworkDnsHostArgs{
					Hostname: pulumi.String("vip"),
					Ip:       pulumi.String("10.200.200.2"),
				},
			},
		},
	}) //, pulumi.Parent(&resource), pulumi.DeleteBeforeReplace(true))
	if err != nil {
		return shared, err
	}
	shared.Network = *network

	cipool, err := libvirt.NewPool(ctx, fmt.Sprintf("%s-cloudinit", state.Uuid()), &libvirt.PoolArgs{
		Type: pulumi.String("dir"),
		Path: pulumi.String(path.Join(poolDir, "cloudinit"))})
	if err != nil {
		return shared, err
	}
	shared.CloudInitStoragePool = *cipool

	vmpool, err := libvirt.NewPool(ctx, fmt.Sprintf("%s-vm", state.Uuid()), &libvirt.PoolArgs{
		Type: pulumi.String("dir"),
		Path: pulumi.String(path.Join(poolDir, "vm"))})
	if err != nil {
		return shared, err
	}
	shared.VmStoragePool = *vmpool

	return shared, nil
}

type LibvirtVm struct {
	pulumi.ResourceState

	Name pulumi.StringOutput `pulumi:"name"`
	IP   pulumi.StringOutput `pulumi:"ip"`

	Unit   *unit.Unit
	Shared *LibvirtShared
	Uuid   *string
}

// Internal type
type unitDepends struct {
	Host     string
	Domain   libvirt.Domain
	Resource pulumi.Resource
}

func Unit(ctx *pulumi.Context, state *cache.State, unit *unit.Unit, shared *LibvirtShared, key *ssh.SshData, inputs *[]filecache.CachedFile) error {
	// unitUuid := uuid.New().String()

	var vmDomains []libvirt.Domain
	var vmDepends []pulumi.Resource
	var fileDepends []pulumi.Resource

	// index 0 vm is treated as special
	var prime unitDepends

	// Assign resources based off their config name later
	unitMap := map[string][]unitDepends{}

	fmt.Printf("unit: %s\n", unit.Name)
	for _, config := range unit.Config {
		fmt.Printf("config: %v\n", config)

		fmt.Printf("%s\n", state)
		cacheSource := path.Join(state.Cache(), config.Qcow2)
		instLink, err := state.RegisterArtifact(fmt.Sprintf("%s/%s.qcow2", unit.Name, config.Name))
		if err != nil {
			return err
		}

		err = os.Link(cacheSource, instLink)
		if err != nil {
			return err
		}

		baseName := fmt.Sprintf("baseimage-%s-%s", unit.Name, config.Name)
		baseVolume, err := libvirt.NewVolume(ctx, baseName, &libvirt.VolumeArgs{
			Name:   pulumi.String(baseName),
			Pool:   shared.VmStoragePool.Name,
			Source: pulumi.Sprintf("file://%s", instLink),
		}, pulumi.Provider(&shared.Provider),
		)

		if err != nil {
			return err
		}

		for idx := 0; idx < config.Count; idx++ {
			hostName := petname.Generate(3, "-")
			//			vmUuid := uuid.New().String()

			ctx.Export(fmt.Sprintf("%s-%s:vm%d", unit.Name, config.Name, idx), pulumi.String(hostName))

			disk1, err := libvirt.NewVolume(ctx, fmt.Sprintf("%s-disk%d", hostName, 1), &libvirt.VolumeArgs{
				Pool:           shared.VmStoragePool.Name,
				BaseVolumeName: baseVolume.Name,
				BaseVolumePool: shared.VmStoragePool.Name,
				Size:           pulumi.Int(config.Disksize),
			}, pulumi.Provider(&shared.Provider),
			)

			if err != nil {
				return err
			}

			userDataFile, err := state.RegisterArtifact(fmt.Sprintf("%s/cloudinit/userdata", hostName))
			if err != nil {
				return err
			}
			metaDataFile, err := state.RegisterArtifact(fmt.Sprintf("%s/cloudinit/metadata", hostName))
			if err != nil {
				return err
			}
			userData, err := cloudinit.UserData(hostName, *key.PubKey)
			err = ioutil.WriteFile(userDataFile, userData, 0600)
			if err != nil {
				return err
			}
			metaData := cloudinit.MetaData()
			err = ioutil.WriteFile(metaDataFile, metaData, 0600)
			if err != nil {
				return err
			}

			cloudInit, err := libvirt.NewCloudInitDisk(ctx, fmt.Sprintf("%s-cloudinit", hostName), &libvirt.CloudInitDiskArgs{
				//				MetaData:      pulumi.String(string(metaData)),
				NetworkConfig: pulumi.String(string(metaData)),
				UserData:      pulumi.String(string(userData)),
				Pool:          shared.CloudInitStoragePool.Name,
			}, pulumi.Provider(&shared.Provider))
			if err != nil {
				return err
			}

			// macaddr, err := vm.Randmac()
			// if err != nil {
			// 	return err
			// }

			domain, err := libvirt.NewDomain(ctx, hostName, &libvirt.DomainArgs{
				Memory:    pulumi.Int(int(config.Memory / vm.MIB)),
				Vcpu:      pulumi.Int(config.Cpu),
				Cloudinit: cloudInit.ID(),
				//				Machine:   pulumi.String("q35"),
				Disks: libvirt.DomainDiskArray{
					libvirt.DomainDiskArgs{
						VolumeId: disk1.ID(),
					},
				},
				NetworkInterfaces: libvirt.DomainNetworkInterfaceArray{
					libvirt.DomainNetworkInterfaceArgs{
						// Mac:          pulumi.String(macaddr),
						Hostname:     pulumi.String(hostName),
						NetworkName:  shared.Network.Name,
						WaitForLease: pulumi.Bool(true), // Need ip's bra
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
				pulumi.Provider(&shared.Provider),
				pulumi.ReplaceOnChanges([]string{"*"}),
				pulumi.DeleteBeforeReplace(true),
			)

			vmDomains = append(vmDomains, *domain)

			ip4 := domain.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))

			//			pulumi.Printf("%s@%s\n", hostName, ip4)
			keyConnectionArgs := remote.ConnectionArgs{
				Host:       ip4,
				User:       pulumi.String("root"),
				PrivateKey: pulumi.Sprintf("%s", *key.PrvKey),
			}

			ensureSshKey, err := remote.NewCommand(ctx, fmt.Sprintf("%s sshkey?", hostName), &remote.CommandArgs{
				Connection: keyConnectionArgs,
				Create:     pulumi.String("uptime"),
			}, pulumi.DependsOn([]pulumi.Resource{domain}),
			)
			if err != nil {
				return err
			}

			remoteRemoteFile := "/usr/local/sbin/remote"

			remoteFile, err := state.RegisterArtifact("bin/remote")
			if err != nil {
				return err
			}
			// Future mitch, don't put this crap into cloud-init
			// again, that like 10x's the cloud-init run time.
			//
			// This is faster by far, if janky af.
			copyRemote, err := remote.NewCopyFile(ctx, fmt.Sprintf("%s copy remote", hostName), &remote.CopyFileArgs{
				Connection: keyConnectionArgs,
				LocalPath:  pulumi.String(remoteFile),
				RemotePath: pulumi.String(remoteRemoteFile),
			}, pulumi.DependsOn([]pulumi.Resource{ensureSshKey}))
			if err != nil {
				return err
			}

			// Note for some os's until you login with a password keys don't work. TODO is figure out why.
			remoteOk, err := remote.NewCommand(ctx, fmt.Sprintf("%s remote?", hostName), &remote.CommandArgs{
				Connection: keyConnectionArgs,
				Create:     pulumi.Sprintf("chmod 755 %s && %s initcheck", remoteRemoteFile, remoteRemoteFile),
			}, pulumi.DependsOn([]pulumi.Resource{ensureSshKey, copyRemote}),
			)
			if err != nil {
				return err
			}
			vmDepends = append(vmDepends, remoteOk)

			vmSetup, err := remote.NewCommand(ctx, fmt.Sprintf("%s /etc/motd", hostName), &remote.CommandArgs{
				Connection: keyConnectionArgs,
				Create:     pulumi.Sprintf("install -m644 /dev/null /etc/motd"),
			}, pulumi.DependsOn([]pulumi.Resource{ensureSshKey}),
			)
			if err != nil {
				return err
			}

			componentName := "shenanigans:libvirt:vm"

			vm := VirtualMachine{
				Name: domain.NetworkInterfaces.Index(pulumi.Int(0)).Hostname().Elem(),
				Ipv4: ip4,
			}

			err = ctx.RegisterComponentResource(componentName, hostName, &vm)
			if err != nil {
				return err
			}

			var ciLogFiles []string
			ciLogFiles = append(ciLogFiles, "cloud-init.log")
			ciLogFiles = append(ciLogFiles, "cloud-init-output.log")

			// abuse ssh to cat log files to a local
			// artifact file, can happen in background and
			// not block other work though should be small.
			for _, log := range ciLogFiles {
				remoteLog := path.Join("/var/log", log)
				localLog, err := state.RegisterArtifact(fmt.Sprintf("%s/cloudinit/%s", hostName, log))
				if err != nil {
					return err
				}
				_, err = local.NewCommand(ctx,
					fmt.Sprintf("%s copy %s", hostName, log),
					&local.CommandArgs{
						// TODO make a function to gennerate the ssh args. But its basically this:
						// ssh -q -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o IdentityFile=sshprivatekey root@ip
						Create: pulumi.Sprintf("ssh -q -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o IdentityFile=%s root@%s 'cat %s' > %s", *key.PrvKeyFile, ip4, remoteLog, localLog),
					},
					pulumi.DependsOn(vmDepends))

				if err != nil {
					return err
				}
			}

			for _, f := range *inputs {
				remoteFile := f.Remote.Dest

				base := filepath.Dir(remoteFile)
				if remoteFile != "" {
					uniq, err := randomString(8)

					if err != nil {
						return err
					}

					cacheFile := path.Join(state.Cache(), f.Sha256Sum)

					uniqTmpFile := path.Join("/tmp", uniq)

					if err != nil {
						return err
					}

					copyInput, err := remote.NewCopyFile(ctx,
						fmt.Sprintf("local:%s->%s", f.Remote.Dest, uniqTmpFile), &remote.CopyFileArgs{
							Connection: keyConnectionArgs,
							LocalPath:  pulumi.String(cacheFile),
							RemotePath: pulumi.String(uniqTmpFile),
						})
					if err != nil {
						return err
					}

					installFile, err := remote.NewCommand(ctx,
						fmt.Sprintf("install %s->%s", uniqTmpFile, remoteFile),
						&remote.CommandArgs{
							Connection: keyConnectionArgs,
							Create:     pulumi.Sprintf("install -dm755 %s && mv %s %s && chown %s:%s %s && chmod %s %s", base, uniqTmpFile, remoteFile, f.Remote.Owner, f.Remote.Group, remoteFile, f.Remote.Mode, remoteFile),
						}, pulumi.DependsOn([]pulumi.Resource{copyInput}))

					if err != nil {
						return err
					}
					// Other stuff depends on the inputs so make sure its there before other things go cray
					fileDepends = append(fileDepends, installFile)
				}
			}

			vmDepends = append(vmDepends, vmSetup)
			vmDepends = append(vmDepends, fileDepends...)

			// Setup the map we'll abuse for k8s kinds and others maybe.
			if config.Name == "default" && idx == 0 {
				fmt.Printf("dbg: prime host %s\n", hostName)
				prime = unitDepends{
					Host:     hostName,
					Domain:   *domain,
					Resource: vmSetup,
				}
			} else {
				fmt.Printf("dbg: adding host %s to config.Name %s\n", hostName, config.Name)
				unitMap[config.Name] = append(unitMap[config.Name], unitDepends{
					Host:     hostName,
					Domain:   *domain,
					Resource: vmSetup,
				})
			}
		}
	}

	// TODO Instead of a ssh config per unit a single one
	// maybe? Future mitch figure this out past mitch is
	// punting on braining and figuring out impact.
	//
	// This is a huge af hack, but whatever it works.
	var sshConfigFileContent pulumi.StringOutput
	sshConfigFileContent = pulumi.Sprintf("%s", "")

	for idx, vm := range vmDomains {
		ip4 := vm.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
		host := vm.NetworkInterfaces.Index(pulumi.Int(0)).Hostname().Elem()

		add := pulumi.All(vm, host, idx, ip4).ApplyT(func(args []interface{}) string {
			return fmt.Sprintf("Host %s vm%d\n  Hostname %s\n  IdentityFile %s\n  User %s\n  UserKnownHostsFile /dev/null\n  StrictHostKeyChecking no\n  LogLevel QUIET\n", args[1], args[2], args[3], *key.PrvKeyFile, "root")
		})
		sshConfigFileContent = pulumi.Sprintf("%s%s", sshConfigFileContent, add)

		// keyConnectionArgs := remote.ConnectionArgs{
		// 	Host:       ip4,
		// 	User:       pulumi.String("root"),
		// 	PrivateKey: pulumi.Sprintf("%s", *key.PrvKey),
		// }

		// _, err := remote.NewCommand(ctx,
		// 	fmt.Sprintf("%s:%s zypper -n dup", unit.Name, host),
		// 	&remote.CommandArgs{
		// 		Connection: keyConnectionArgs,
		// 		Create:     pulumi.Sprintf("zypper -n dup"),
		// 	}, pulumi.DependsOn(vmDepends),
		// )
		// if err != nil {
		// 	return err
		// }
	}

	// Only write the config file if all the vm's came up,
	// if not its likely system is OOM or ENOSPC or who
	// knows but no point in writing this out at this point.
	sshConfigFile, err := state.RegisterArtifact(fmt.Sprintf("%s/ssh/config", unit.Name))
	if err != nil {
		return err
	}
	_, err = local.NewCommand(ctx,
		fmt.Sprintf("%s:ssh-config", unit.Name),
		&local.CommandArgs{
			Create: pulumi.Sprintf("echo '%s' > %s", sshConfigFileContent, sshConfigFile),
		},
		pulumi.DependsOn(vmDepends),
	)

	// k8s kind setup work
	k8sDepends := vmDepends

	// Setup dag for k8s kind work
	if unit.Kind == "k8s" {
		// Setup the prime node first
		primeip4 := prime.Domain.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
		// primehost := prime.Domain.NetworkInterfaces.Index(pulumi.Int(0)).Hostname().Elem()
		keyConnectionArgs := remote.ConnectionArgs{
			Host:       primeip4,
			User:       pulumi.String("root"),
			PrivateKey: pulumi.Sprintf("%s", *key.PrvKey),
		}

		k8sInstall, err := remote.NewCommand(ctx,
			fmt.Sprintf("%s:%s k8s prime", unit.Name, prime.Host),
			&remote.CommandArgs{
				Connection: keyConnectionArgs,
				Create:     pulumi.Sprintf("%s k8s --prime", "/usr/local/sbin/remote"),
			}, pulumi.DependsOn(k8sDepends),
		)
		if err != nil {
			return err
		}

		// Everyone depends on the prime install being setup and Ready
		k8sDepends = append(k8sDepends, k8sInstall)

		// Copy over the kubeconfig file from the first node into artifacts
		localKubeconfig, err := state.RegisterArtifact(fmt.Sprintf("/%s/kube/config", unit.Name))
		if err != nil {
			return err
		}

		remoteKubeconfig := "/etc/rancher/rke2/rke2.yaml"

		// TODO setup a provider using the file and return that as an output someday
		_, err = local.NewCommand(ctx,
			fmt.Sprintf("%s copy kubeconfig", unit.Name),
			&local.CommandArgs{
				Create: pulumi.Sprintf("ssh -q -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o IdentityFile=%s root@%s 'cat %s | sed -e \"s/127.0.0.1/%s/\"' > %s && chmod 600 %s", *key.PrvKeyFile, primeip4, remoteKubeconfig, primeip4, localKubeconfig, localKubeconfig),
			}, pulumi.DependsOn(k8sDepends))

		if err != nil {
			return err
		}

		//		fmt.Printf("len of unit %s agents is %d\n", unit.Name, len(unitMap["agent"]))

		// The rest of the control-plane/admin/server nodes (if any)
		var defaultInstalls []pulumi.Resource
		for _, vm := range unitMap["default"] {
			fmt.Printf("agent:%s:%s", unit.Name, prime.Host)
			ip4 := vm.Domain.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
			// primehost := prime.Domain.NetworkInterfaces.Index(pulumi.Int(0)).Hostname().Elem()
			fmt.Printf("unit %s host %s\n", unit.Name, vm.Host)
			keyConnectionArgs := remote.ConnectionArgs{
				Host:       ip4,
				User:       pulumi.String("root"),
				PrivateKey: pulumi.Sprintf("%s", *key.PrvKey),
			}
			anadmin, err := remote.NewCommand(ctx,
				fmt.Sprintf("%s:%s k8s admin", unit.Name, vm.Host),
				&remote.CommandArgs{
					Connection: keyConnectionArgs,
					Create:     pulumi.Sprintf("%s k8s --upstream %s", "/usr/local/sbin/remote", primeip4),
				}, pulumi.DependsOn(k8sDepends),
			)
			defaultInstalls = append(defaultInstalls, anadmin)

			if err != nil {
				return err
			}
		}

		var workerInstalls []pulumi.Resource
		var pkgInstalls []pulumi.Resource
		// Wait for the node to become Ready first.
		// Kick off the agent installs next, we'll do control plane nodes last
		for _, vm := range unitMap["agent"] {
			fmt.Printf("agent:%s:%s", unit.Name, prime.Host)
			ip4 := vm.Domain.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
			// primehost := prime.Domain.NetworkInterfaces.Index(pulumi.Int(0)).Hostname().Elem()
			fmt.Printf("unit %s host %s\n", unit.Name, vm.Host)
			keyConnectionArgs := remote.ConnectionArgs{
				Host:       ip4,
				User:       pulumi.String("root"),
				PrivateKey: pulumi.Sprintf("%s", *key.PrvKey),
			}
			aworker, err := remote.NewCommand(ctx,
				fmt.Sprintf("%s:%s k8s worker", unit.Name, vm.Host),
				&remote.CommandArgs{
					Connection: keyConnectionArgs,
					Create:     pulumi.Sprintf("%s k8s --worker --upstream %s", "/usr/local/sbin/remote", primeip4),
				}, pulumi.DependsOn(k8sDepends),
			)
			workerInstalls = append(workerInstalls, aworker)

			if err != nil {
				return err
			}

			if unit.Online {
				zypperInOpenIscsi, err := remote.NewCommand(ctx,
					fmt.Sprintf("%s:%s zypper -n in open-iscsi", unit.Name, vm.Host),
					&remote.CommandArgs{
						Connection: keyConnectionArgs,
						Create:     pulumi.String("zypper -n in open-iscsi"),
					}, pulumi.DependsOn(vmDepends))
				if err != nil {
					return err
				}
				pkgInstalls = append(pkgInstalls, zypperInOpenIscsi)

				// TODO migrate all this into the
				// remote command, this can fail with
				// errno7 cause ^^^ keeps locking the
				// dam database, push the retries down
				// to the remote command.
				//
				// zypperDup, err := remote.NewCommand(ctx,
				// 	fmt.Sprintf("%s:%s zypper -n dup", unit.Name, vm.Host),
				// 	&remote.CommandArgs{
				// 		Connection: keyConnectionArgs,
				// 		Create:     pulumi.String("zypper -n dup"),
				// 	}, pulumi.DependsOn(pkgInstalls))
				// if err != nil {
				// 	return err
				// }
				// pkgInstalls = append(pkgInstalls, zypperDup)
			}

		}

		vmDepends = append(vmDepends, k8sDepends...)
		vmDepends = append(vmDepends, defaultInstalls...)
		vmDepends = append(vmDepends, workerInstalls...)
		vmDepends = append(vmDepends, pkgInstalls...)
	}

	// Do online related stuff.
	if unit.Online {
		localKubeconfig, err := state.RegisterArtifact(fmt.Sprintf("/%s/kube/config", unit.Name))
		if err != nil {
			return err
		}
		if unit.Name == "upstream" {
			helmfileApply, err := local.NewCommand(ctx,
				fmt.Sprintf("%s helmfile apply", unit.Name),
				&local.CommandArgs{
					Create: pulumi.Sprintf("cd helm && env KUBECONFIG=%s helmfile apply", localKubeconfig),
				}, pulumi.DependsOn(vmDepends))

			if err != nil {
				return err
			}
			vmDepends = append(vmDepends, helmfileApply)
		}
	}

	return nil
}
