// vms component group, all virtual machines that are created are in a
// "group" even if that group is 0 or 1 in size.
package vms

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	ssh "deploy/pkg/ssh"
	// for later...
	//	flag "github.com/spf13/pflag"
	//	b64 "encoding/base64"

	// cloudinit "github.com/pulumi/pulumi-cloudinit/sdk"

	"github.com/adrg/xdg"
	"github.com/dustinkirkland/golang-petname"
	"github.com/google/uuid"
	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi-libvirt/sdk/go/libvirt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	// tls "github.com/pulumi/pulumi-tls/sdk/v4/go/tls"
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

// func createPool(ctx *pulumi.Context, name string) (*libvirt.Pool, error) {
// 	// `pool` is a storage pool that can be used to create volumes
// 	// the `dir` type uses a directory to manage files
// 	// `Path` maps to a directory on the host filesystem, so we'll be able to
// 	// volume contents in `/pool/cluster_storage/`
// 	pool, err := libvirt.NewPool(ctx, fmt.Sprintf("%s-pool", name), &libvirt.PoolArgs{
// 		Type: pulumi.String("dir"),
// 		Path: pulumi.String(fmt.Sprintf("/var/tmp/shenanigans/%s/", name)),
// 	}) //, pulumi.Parent(&resource), pulumi.DeleteBeforeReplace(true))
// 	if err == nil {
// 		return new(libvirt.Pool), err

// 	}
// 	return pool, nil

// }

func setupResult(dir string) (string, error) {
	return dir, os.MkdirAll(dir, 0755)
}

type SshData struct {
	PrvKey     *string
	PubKey     *string
	PrvKeyFile *string
	PubKeyFile *string
}

// Sets up the ssh ca resources and returns things a vm will need to setup/sign
// its host keys.
func SetupSshCa(ctx *pulumi.Context, nameSuffix string, dir string) (SshData, error) {
	// Base dir for all results is dir/ssh to namespace things.
	prefix := path.Join(dir, "ssh")
	os.MkdirAll(prefix, 0755)

	key, err := ssh.NewPrivateRsaKey(2048)
	if err != nil {
		return SshData{}, err
	}

	privateKeyFile := path.Join(prefix, "id_rsa")
	privateKeyData := ssh.EncodePrivateKeyToPEM(key)
	ctx.Export("caPrv", pulumi.String(privateKeyFile))
	err = ssh.WriteSshkeyFile(privateKeyData, privateKeyFile)
	if err != nil {
		return SshData{}, err
	}

	publicKeyFile := fmt.Sprintf("%s.pub", privateKeyFile)
	publicKeyData, err := ssh.NewPublicRsaKey(&key.PublicKey)
	if err != nil {
		return SshData{}, err
	}
	err = ssh.WriteSshkeyFile(publicKeyData, publicKeyFile)
	if err != nil {
		return SshData{}, err
	}
	ctx.Export("caPub", pulumi.String(publicKeyFile))

	prv := string(privateKeyData)
	pub := string(publicKeyData)

	// Trim the trailing newline
	prv = strings.TrimSuffix(prv, "\n")
	pub = strings.TrimSuffix(pub, "\n")

	return SshData{
		PrvKey:     &prv,
		PubKey:     &pub,
		PrvKeyFile: &privateKeyFile,
		PubKeyFile: &publicKeyFile,
	}, nil
}

type LibvirtVm struct {
	pulumi.ResourceState

	Name pulumi.StringOutput `pulumi:"name"`
	IP   pulumi.StringOutput `pulumi:"ip"`
}

func createVirtualMachines(ctx *pulumi.Context) error {
	var vmConfig VMConfig
	var vms []VM
	var userdata UserData
	var instanceUuid string
	var libvirtConfig LibVirtConfig

	instanceUuid = uuid.New().String()

	ctx.Export("instance_uuid", pulumi.String(instanceUuid))

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	stack := ctx.Stack()

	stackResultDir, err := setupResult(path.Join(dir, "artifacts", stack))
	if err != nil {
		return err
	}
	ctx.Export("stackResultDir", pulumi.String(stackResultDir))

	suffix := instanceUuid

	sshKey, err := SetupSshCa(ctx, suffix, stackResultDir)
	if err != nil {
		return err
	}

	cfg := config.New(ctx, "shenanigans")
	cfg.RequireObject("vmconfig", &vmConfig)
	cfg.RequireObject("vms", &vms)
	//	cfg.RequireObject("users", &userdata.Users)

	libvirtUri := "qemu://system"
	provider, err := libvirt.NewProvider(ctx, "provider", &libvirt.ProviderArgs{
		Uri: pulumi.String(libvirtUri),
	})
	if err != nil {
		return err
	}

	ctx.Export("provider", pulumi.String("libvirt"))
	ctx.Export("libvirt", pulumi.String(libvirtUri))

	poolDir := "/var/lib/libvirt/shenanigans"
	poolInstDir := path.Join(poolDir, instanceUuid)

	// Cover removal of the artifact dir for the stack directory
	_, err = local.NewCommand(ctx,
		fmt.Sprintf("%s-artifact", instanceUuid),
		&local.CommandArgs{
			Create: pulumi.Sprintf("install -dm755 %s", path.Join(dir, "artifacts", stack)),
			Delete: pulumi.Sprintf("rm -fr %s", path.Join(dir, "artifacts", stack)),
		},
	)

	cloudInitStoragePool, err := libvirt.NewPool(ctx, fmt.Sprintf("%s-cloudinit", instanceUuid), &libvirt.PoolArgs{
		Type: pulumi.String("dir"),
		Path: pulumi.String(path.Join(poolInstDir, "cloudinit"))})
	if err != nil {
		return err
	}

	vmStoragePool, err := libvirt.NewPool(ctx, fmt.Sprintf("%s-vm", instanceUuid), &libvirt.PoolArgs{
		Type: pulumi.String("dir"),
		Path: pulumi.String(path.Join(poolInstDir, "vm"))})
	if err != nil {
		return err
	}

	cfg.RequireObject("libvirt", &libvirtConfig)
	// Create a hard link to:
	// ~/.cache/shenanigans/cache/SHA256SUM
	// from:
	// ~/.cache/shenanigans/instances/INSTANCEUUID.qcow2

	// Note that the cachefile is done entirely outside of pulumi
	// config so it can be used across all instances that might
	// get built between stacks. Saves on downloading crap constantly.
	cachePrefix := path.Join(xdg.CacheHome, "shenanigans", "cache")
	instPrefix := path.Join(xdg.CacheHome, "shenanigans", "instances")
	goArtDir := path.Join(stackResultDir, "go")

	os.MkdirAll(cachePrefix, 0755)
	os.MkdirAll(instPrefix, 0755)
	os.MkdirAll(goArtDir, 0755)

	cacheFile := path.Join(cachePrefix, libvirtConfig.Qcow2)
	instFile := path.Join(instPrefix, fmt.Sprintf("%s.qcow2", instanceUuid))
	err = os.Link(cacheFile, instFile)

	if err != nil {
		_ = fmt.Errorf(err.Error())
		return err
	}

	// TODOwhy doesn't this always cleanup stuff?
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() error {
		<-sigs
		_ = os.Remove(instFile)
		return nil
	}()

	remoteFile := path.Join(goArtDir, "remote")

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

		log.Fatalf("cmd.Run() failed with %s\n", err)
		return err
	}

	ctx.Export("remote", pulumi.String(remoteFile))

	baseName := "base-opensuse"
	baseVolName := pulumi.Sprintf("%s-%s", baseName, instanceUuid)
	baseVolume, err := libvirt.NewVolume(ctx, baseName, &libvirt.VolumeArgs{
		Name:   baseVolName,
		Pool:   vmStoragePool.Name,
		Source: pulumi.Sprintf("file://%s", instFile),
	}, pulumi.Provider(provider),
	)

	if err != nil {
		_ = fmt.Errorf(err.Error())
		return err
	}

	network, err := libvirt.NewNetwork(ctx, instanceUuid, &libvirt.NetworkArgs{
		// TODO: address range selection for ipv4/6? future me problem
		Name:      pulumi.String(instanceUuid),
		Addresses: pulumi.StringArray{pulumi.String("10.200.200.1/24")},
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
		_ = fmt.Errorf(err.Error())
		return err
	}

	var vmDomains []libvirt.Domain

	var vmDepends []pulumi.Resource

	for idx, vm := range vms {
		vm.Network = vmConfig.Network

		if vm.Network.Interface == "" {
			vm.Network.Interface = "eth0"
		}

		hostName := petname.Generate(3, "-")

		ctx.Export(fmt.Sprintf("vm%d", idx), pulumi.String(hostName))

		//		vmUuid := uuid.New().String()
		vmName := hostName
		userdata.Hostname = hostName
		userdata.Sshkey = *sshKey.PubKey

		fileSystem, err := libvirt.NewVolume(ctx, fmt.Sprintf("%s-%d-disk%d", instanceUuid, idx, 1), &libvirt.VolumeArgs{
			Pool:           vmStoragePool.Name,
			BaseVolumeName: baseVolume.Name,
			BaseVolumePool: vmStoragePool.Name,
			Size:           pulumi.Int(GB * vmConfig.DiskSize),
		}, pulumi.Provider(provider),
		)

		if err != nil {
			return err
		}

		fileSystem2, err := libvirt.NewVolume(ctx, fmt.Sprintf("%s-%d-disk%d", instanceUuid, idx, 2), &libvirt.VolumeArgs{
			Pool:           vmStoragePool.Name,
			BaseVolumeName: baseVolume.Name,
			BaseVolumePool: vmStoragePool.Name,
			Size:           pulumi.Int(GB * vmConfig.DiskSize),
		}, pulumi.Provider(provider),
		)

		if err != nil {
			return err
		}

		fileSystem3, err := libvirt.NewVolume(ctx, fmt.Sprintf("%s-%d-disk%d", instanceUuid, idx, 3), &libvirt.VolumeArgs{
			Pool:           vmStoragePool.Name,
			BaseVolumeName: baseVolume.Name,
			BaseVolumePool: vmStoragePool.Name,
			Size:           pulumi.Int(GB * vmConfig.DiskSize),
		}, pulumi.Provider(provider),
		)

		if err != nil {
			return err
		}

		// userdata.Hostname = petName.ID().ToStringOutput().ApplyT(func(s string) string {
		// 	fmt.Printf(s)
		// 	return s
		// })
		// userdata.Hostname = fmt.Sprintf("%s", petName.ID().ToStringOutput().ApplyT(func(s string) string {
		// 	return s
		// }))
		// userdata.Hostname = fmt.Sprintf("%s", pulumi.Sprintf("%s", computerName))

		// petName.ID().ToStringOutput().ApplyT(func(id string) string {
		// 	userdata.Hostname = string(id)
		// 	fmt.Printf("wtf inside: %d is allegedly: %s\n", idx, id)
		// 	return fmt.Sprintf("petname-%d", idx)
		// })

		// config, err := cloudinit.NewConfig(ctx, "admin", &cloudinit.ConfigArgs{
		// 	Gzip:         pulumi.Bool(false),
		// 	Base64Encode: pulumi.Bool(false),
		// 	Parts: &cloudinit.ConfigPartArray{
		// 		// &cloudinit.ConfigPartArgs{
		// 		// 	ContentType: pulumi.String("text/x-shellscript"),
		// 		// 	Content:     pulumi.String(adminUserDataSetup),
		// 		// },
		// 		&cloudinit.ConfigPartArgs{
		// 			ContentType: pulumi.String("text/cloud-config"),
		// 			Content:     pulumi.String(adminUserData1),
		// 		},
		// 		// &cloudinit.ConfigPartArgs{
		// 		// 	ContentType: pulumi.String("text/x-shellscript"),
		// 		// 	Content:     pulumi.String(adminUserData2),
		// 		// },
		// 	},
		// })

		// if err != nil {
		// 	return nil, err
		// }

		// r, err := os.ReadFile(remoteFile)
		// if err != nil {
		// 	return err
		// }

		// rfb64 := b64.StdEncoding.EncodeToString([]byte(r))

		// userdata.RemoteData = rfb64
		//		userdata.Sshkey = *sshKey.PubKey

		ciArtDir := path.Join(stackResultDir, "cloudinit", fmt.Sprintf("vm%d", idx))
		os.MkdirAll(ciArtDir, 0755)

		cloudInitUserData, err := ParseTemplate("./cloud_init_user_data.yaml.gotmpl", userdata)
		if err != nil {
			return err
		}

		cloudInitMetaData, err := ParseTemplate("./cloud_init_meta_data.yaml.gotmpl", vm)
		if err != nil {
			return err
		}

		userData := path.Join(ciArtDir, "user_data")
		err = ioutil.WriteFile(userData, []byte(cloudInitUserData), 0600)
		if err != nil {
			return err
		}
		metaData := path.Join(ciArtDir, "meta_data")
		err = ioutil.WriteFile(metaData, []byte(cloudInitMetaData), 0600)
		if err != nil {
			return err
		}

		cloudInit, err := libvirt.NewCloudInitDisk(ctx, fmt.Sprintf("%s-%d-cloudinit", instanceUuid, idx), &libvirt.CloudInitDiskArgs{
			MetaData:      pulumi.String(string(cloudInitMetaData)),
			NetworkConfig: pulumi.String(string(cloudInitMetaData)),
			UserData:      pulumi.String(string(cloudInitUserData)),
			Pool:          cloudInitStoragePool.Name,
		}, pulumi.Provider(provider))
		if err != nil {
			return err
		}

		lvDomain, err := libvirt.NewDomain(ctx, vmName, &libvirt.DomainArgs{
			Memory:    pulumi.Int(vmConfig.Memory * 1024),
			Vcpu:      pulumi.Int(vmConfig.Cpu),
			Cloudinit: cloudInit.ID(),
			Disks: libvirt.DomainDiskArray{
				libvirt.DomainDiskArgs{
					VolumeId: fileSystem.ID(),
				},
				libvirt.DomainDiskArgs{
					VolumeId: fileSystem2.ID(),
				},
				libvirt.DomainDiskArgs{
					VolumeId: fileSystem3.ID(),
				},
			},
			NetworkInterfaces: libvirt.DomainNetworkInterfaceArray{
				libvirt.DomainNetworkInterfaceArgs{
					NetworkName:  network.Name,
					Hostname:     pulumi.String(vmName),
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

		keyConnectionArgs := remote.ConnectionArgs{
			Host:       lvDomain.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0)),
			User:       pulumi.String("root"),
			PrivateKey: pulumi.Sprintf("%s", *sshKey.PrvKey),
		}

		// Note for some os's until you login with a password keys don't work. TODO is figure out why.
		ensureSshKey, err := remote.NewCommand(ctx, fmt.Sprintf("%s ssh pass?", vmName), &remote.CommandArgs{
			Connection: keyConnectionArgs,
			Create:     pulumi.String("uptime"),
		}, pulumi.DependsOn([]pulumi.Resource{lvDomain}),
		)
		if err != nil {
			return err
		}

		remoteRemoteFile := "/usr/local/sbin/remote"
		// Future mitch, don't put this crap into cloud-init
		// again, that like 10x's the cloud-init run time.
		//
		// This is faster by far, if janky af.
		copyRemote, err := remote.NewCopyFile(ctx, fmt.Sprintf("vm%d copy remote", idx), &remote.CopyFileArgs{
			Connection: keyConnectionArgs,
			LocalPath:  pulumi.String(remoteFile),
			RemotePath: pulumi.String(remoteRemoteFile),
		}, pulumi.DependsOn([]pulumi.Resource{ensureSshKey}))
		if err != nil {
			return err
		}

		// Note for some os's until you login with a password keys don't work. TODO is figure out why.
		remoteOk, err := remote.NewCommand(ctx, fmt.Sprintf("%s remote ok?", vmName), &remote.CommandArgs{
			Connection: keyConnectionArgs,
			Create:     pulumi.Sprintf("chmod 755 %s && %s initcheck", remoteRemoteFile, remoteRemoteFile),
		}, pulumi.DependsOn([]pulumi.Resource{ensureSshKey, copyRemote}),
		)
		if err != nil {
			return err
		}
		vmDepends = append(vmDepends, remoteOk)

		vmSetup, err := remote.NewCommand(ctx, fmt.Sprintf("%s motd?", vmName), &remote.CommandArgs{
			Connection: keyConnectionArgs,
			Create:     pulumi.Sprintf("install -m644 /dev/null /etc/motd"),
		}, pulumi.DependsOn([]pulumi.Resource{ensureSshKey}),
		)
		if err != nil {
			return err
		}

		vmDomains = append(vmDomains, *lvDomain)
		vmDepends = append(vmDepends, vmSetup)
	}

	//	var sshConfigRequired []pulumi.Resource
	var sshConfigFileContent pulumi.StringOutput
	sshConfigFileContent = pulumi.Sprintf("%s", "")

	for idx, vm := range vmDomains {
		ip4 := vm.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
		// host := vm.NetworkInterfaces.Index(pulumi.Int(0)).Hostname()
		host := vm.Name
		//		uuid := vm.Name

		add := pulumi.All(vm, host, idx, ip4).ApplyT(func(args []interface{}) string {
			return fmt.Sprintf("Host %s vm%d\n  Hostname %s\n  IdentityFile %s\n  User %s\n  UserKnownHostsFile /dev/null\n  StrictHostKeyChecking no\n  LogLevel QUIET\n", args[1], args[2], args[3], *sshKey.PrvKeyFile, "root")
		})
		sshConfigFileContent = pulumi.Sprintf("%s%s", sshConfigFileContent, add)
		//		sshConfigRequired = append(sshConfigRequired, vm)
	}

	sshConfigFile := path.Join(stackResultDir, "ssh", "config")
	sshConfigFileWriter, err := local.NewCommand(ctx,
		"ssh-config",
		&local.CommandArgs{
			Create: pulumi.Sprintf("echo '%s' > %s", sshConfigFileContent, sshConfigFile),
			Delete: pulumi.Sprintf("rm -f %s", sshConfigFile),
		},
		//		pulumi.DependsOn(sshConfigRequired),
	)

	ctx.Export("sshconfig", pulumi.String(sshConfigFile))

	ciLogName := "cloud-init-output.log"
	ciRemoteLogFile := path.Join("/var/log", ciLogName)

	for idx, _ := range vmDomains {
		vmHost := fmt.Sprintf("vm%d", idx)
		ciArtDir := path.Join(stackResultDir, "cloudinit", vmHost)
		os.MkdirAll(ciArtDir, 0755)

		ciLogFile := path.Join(ciArtDir, ciLogName)

		ciLogDepends := vmDepends
		ciLogDepends = append(ciLogDepends, sshConfigFileWriter)

		_, err = local.NewCommand(ctx,
			fmt.Sprintf("vm%d copy cloud-init log", idx),
			&local.CommandArgs{
				Create: pulumi.Sprintf("ssh -F %s %s 'cat %s' > %s", sshConfigFile, vmHost, ciRemoteLogFile, ciLogFile),
			},
			pulumi.DependsOn(ciLogDepends))

		if err != nil {
			return err
		}
	}

	// // TODO move this crap into some sorta higher level grouping of shit
	// for idx, vm := range vmDomains {
	// 	ip4 := vm.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
	// 	host := vm.Name

	// 	// Abusing the instance uuid to act as the rke2 join
	// 	// token, sue me
	// 	rke2token := instanceUuid

	// 	// Copy in the cached rke2 artifacts that the remote
	// 	// binary needs to the dirs they are expected to be in
	// 	add := pulumi.All(vm, host, idx, ip4).ApplyT(func(args []interface{}) string {
	// 		return fmt.Sprintf("Host %s vm%d\n  Hostname %s\n  IdentityFile %s\n  User %s\n  UserKnownHostsFile /dev/null\n  StrictHostKeyChecking no\n  LogLevel QUIET\n", args[1], args[2], args[3], *sshKey.PrvKeyFile, "root")
	// 	})
	// 	sshConfigFileContent = pulumi.Sprintf("%s%s", sshConfigFileContent, add)
	// 	//		sshConfigRequired = append(sshConfigRequired, vm)
	// }

	var inputs []CacheFile
	cfg.RequireObject("inputs", &inputs)

	for idx, vm := range vmDomains {
		ip4 := vm.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
		keyConnectionArgs := remote.ConnectionArgs{
			Host:       ip4,
			User:       pulumi.String("root"),
			PrivateKey: pulumi.Sprintf("%s", *sshKey.PrvKey),
		}

		//		vmHost := fmt.Sprintf("vm%d", idx)
		for _, f := range inputs {
			cacheFile := path.Join(cachePrefix, f.Sha256Sum)
			remoteFile := f.Remote.Dest
			remoteFileTmp := fmt.Sprintf("%s.tmp", f.Remote.Dest)

			base := filepath.Dir(remoteFile)
			if remoteFile != "" {
				pulumi.Sprintf("vm%d input: %s -> %s\n", idx, cacheFile, remoteFile)

				baseDir, err := remote.NewCommand(ctx,
					fmt.Sprintf("vm%d basedir %s", idx, remoteFile),
					&remote.CommandArgs{
						Connection: keyConnectionArgs,
						Create:     pulumi.Sprintf("mkdir -p %s || :", base),
					}, pulumi.DependsOn([]pulumi.Resource{sshConfigFileWriter}),
				)

				if err != nil {
					return err
				}

				copyInput, err := remote.NewCopyFile(ctx, fmt.Sprintf("vm%d copy %s", idx, remoteFile), &remote.CopyFileArgs{
					Connection: keyConnectionArgs,
					LocalPath:  pulumi.String(cacheFile),
					RemotePath: pulumi.String(remoteFileTmp),
				}, pulumi.DependsOn([]pulumi.Resource{baseDir}))
				if err != nil {
					return err
				}

				installFile, err := remote.NewCommand(ctx,
					fmt.Sprintf("vm%d copy %s", idx, remoteFile),
					&remote.CommandArgs{
						Connection: keyConnectionArgs,
						Create:     pulumi.Sprintf("install -m%s --owner %s --group %s %s %s && rm %s", f.Remote.Mode, f.Remote.Owner, f.Remote.Group, remoteFileTmp, remoteFile, remoteFileTmp),
					}, pulumi.DependsOn([]pulumi.Resource{copyInput}))

				if err != nil {
					return err
				}
				vmDepends = append(vmDepends, installFile)
			}
		}
	}

	primeip4 := pulumi.StringOutput{}

	var k8sDepends []pulumi.Resource

	for idx, vm := range vmDomains {
		ip4 := vm.NetworkInterfaces.Index(pulumi.Int(0)).Addresses().Index(pulumi.Int(0))
		keyConnectionArgs := remote.ConnectionArgs{
			Host:       ip4,
			User:       pulumi.String("root"),
			PrivateKey: pulumi.Sprintf("%s", *sshKey.PrvKey),
		}

		vmHost := fmt.Sprintf("vm%d", idx)

		if idx == 0 {
			primeip4 = ip4
			ctx.Export("primeip4", primeip4)
			k8sInstall, err := remote.NewCommand(ctx,
				fmt.Sprintf("vm%d k8s prime", idx),
				&remote.CommandArgs{
					Connection: keyConnectionArgs,
					Create:     pulumi.Sprintf("%s k8s --prime", "/usr/local/sbin/remote"),
				}, pulumi.DependsOn(vmDepends),
			)

			if err != nil {
				return err
			}

			k8sDepends = append(k8sDepends, k8sInstall)
			remoteKubeconfig := "/etc/rancher/rke2/rke2.yaml"

			k8sStackDir := path.Join(stackResultDir, "k8s", vmHost)
			localKubeconfig := path.Join(k8sStackDir, "kubeconfig")

			os.MkdirAll(k8sStackDir, 0755)

			_, err = local.NewCommand(ctx,
				fmt.Sprintf("vm%d copy kubeconfig", idx),
				&local.CommandArgs{
					Create: pulumi.Sprintf("ssh -F %s %s 'cat %s | sed -e \"s/127.0.0.1/%s/\"' > %s", sshConfigFile, vmHost, remoteKubeconfig, primeip4, localKubeconfig),
				},
				pulumi.DependsOn([]pulumi.Resource{sshConfigFileWriter, k8sInstall}))

			if err != nil {
				return err
			}
		} else {
			_, err = remote.NewCommand(ctx,
				fmt.Sprintf("vm%d k8s worker", idx),
				&remote.CommandArgs{
					Connection: keyConnectionArgs,
					Create:     pulumi.Sprintf("%s k8s --worker --upstream %s", "/usr/local/sbin/remote", primeip4),
				}, pulumi.DependsOn(k8sDepends),
			)

			if err != nil {
				return err
			}
		}
	}
	return nil
}
