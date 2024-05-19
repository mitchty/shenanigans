package remote

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	//	"deploy/pkg/components/groups"
	// "github.com/mitchty/shenanigans/pkg/cache"
	flag "github.com/spf13/pflag"
)

func Cmd() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "initcheck": // This just exists to validate the binary after scp
			fmt.Printf("ok\n")
			os.Exit(0)
		case "hostname":
			// This is just silly ignore it
			hostname, err := os.Hostname()
			if err != nil {
				fmt.Println(err)
				return err
			}
			fmt.Printf("%s\n", hostname)
		case "k8s": // k8s subcommand hard coded to rke2 for now
			if len(os.Args) > 2 {
				switch os.Args[2] {
				default: // TODO--flavor=rke2|k3s|?  etc... for now rke2 airgap only and all defaults future mitch can figure out how the hell I do customization or not.
					// Args for k8s for now are --prime which means initial (prime) node
					// --join ip
					// --token string filled in by pulumi config to be the uuid of the instance for the prime node and the other nodes
					// TODOworker(s)
					prime := flag.Bool("prime", false, "Is this the first control plane node?")
					worker := flag.Bool("worker", false, "Is this node a worker node?")
					upstream := flag.String("upstream", "", "For all non prime nodes the upstream ip/host to join a cluster")

					flag.Parse()

					// Setup sysctls needed for kubelet to run
					kubeletSysctlFile := "/etc/sysctl.d/90-kubelet.conf"
					fmt.Printf("%s\n", kubeletSysctlFile)

					kubeletSysctlData := []byte(`vm.panic_on_oom=0
vm.overcommit_memory=1
kernel.panic=10
kernel.panic_on_oops=1
kernel.keys.root_maxkeys=1000000
kernel.keys.root_maxbytes=25000000
net.ipv6.conf.all.disable_ipv6=1
net.ipv4.ip_forward=1
net.ipv4.conf.all.forwarding=1
net.ipv4.conf.default.forwarding=1
# net.nf_conntrack_max=131072
# net.netfilter.nf_conntrack_tcp_timeout_established=8640
# net.netfilter.nf_conntrack_tcp_timeout_close_wait=3600
`)

					err := ioutil.WriteFile(kubeletSysctlFile, kubeletSysctlData, 0600)
					if err != nil {
						return err
					}

					var stdout, stderr bytes.Buffer

					// Ensure they are setup prior to installing and for reboots
					cmd := exec.Command("sysctl", "-p", kubeletSysctlFile)
					cmd.Env = os.Environ()
					cmd.Stdout = &stdout
					cmd.Stderr = &stderr
					err = cmd.Run()
					if err != nil {
						outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
						fmt.Printf("out:\n%s\nerr:\n%s\n", outStr, errStr)
						fmt.Printf("err is: %s\n", err)
						return err
					}

					rke2Dir := "/etc/rancher/rke2"
					err = os.MkdirAll(rke2Dir, 0755)
					if err != nil {
						return err
					}

					rke2ConfigFile := path.Join(rke2Dir, "config.yaml")

					fmt.Printf("%s\n", rke2ConfigFile)

					commonConfig := []byte(`---
token: bootstraptoken
`)
					//					cni: cilium

					rke2Config := commonConfig
					if !*worker {

						rke2Sus := []byte(`# kube-controller-manager-arg:
# - bind-address=127.0.0.1
# - use-service-account-credentials=true
# - tls-min-version=VersionTLS12
# - tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
# kube-scheduler-arg:
# - tls-min-version=VersionTLS12
# - tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
# kube-apiserver-arg:
# - tls-min-version=VersionTLS12
# - tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
# - authorization-mode=RBAC,Node
# - anonymous-auth=false
# - audit-log-mode=blocking-strict
# - audit-log-maxage=30
# - read-only-port=0
# - authorization-mode=Webhook
`)
						fmt.Sprintf("%s", rke2Sus) //sue me go compiler eat a dick

						rke2Rest := []byte(`write-kubeconfig-mode: "0644"
selinux: false
disable:
#- rke2-ingress-nginx
- rke2-snapshot-validation-webhook
- rke2-snapshot-controller
- rke2-snapshot-controller-crd
`)
						rke2Debug := []byte(`debug: true
kubelet-arg:
- "protect-kernel-defaults=true"
- alsologtostderr=true
- v=4
`)
						fmt.Sprintf("%v\n", rke2Debug)

						// hostname, err := os.Hostname()
						// if err != nil {
						// 	fmt.Println(err)
						// 	return err
						// }
						// rke2Tls := []byte(fmt.Sprintf("tls-san:\n  - %s.dev.home.arpa\n", hostname))
						rke2Tls := []byte(fmt.Sprintf("tls-san:\n  - 10.200.200.2\n  - vip.dev.home.arpa\n"))

						// kubelet-arg:
						// - protect-kernel-defaults=true
						rke2Config = append(rke2Config, rke2Rest...)
						rke2Config = append(rke2Config, rke2Tls...)
					}

					// Everyone but the prime node goes to the prime node for setup
					if !*prime {
						serverConf := []byte(fmt.Sprintf("server: https://%s:9345\n", *upstream))
						rke2Config = append(rke2Config, serverConf...)
					}

					// Write out a profile.d file to add to $PATH
					err = ioutil.WriteFile(rke2ConfigFile, rke2Config, 0600)
					if err != nil {
						return err
					}

					fmt.Printf("rke2 install.sh\n")
					cmd = exec.Command("/tmp/rke2/install.sh")
					cmd.Env = os.Environ()
					cmd.Env = append(cmd.Env, "INSTALL_RKE2_ARTIFACT_PATH=/tmp/rke2")

					if *worker {
						cmd.Env = append(cmd.Env, "INSTALL_RKE2_TYPE=agent")
					}

					cmd.Stdout = &stdout
					cmd.Stderr = &stderr
					err = cmd.Run()
					if err != nil {
						outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
						fmt.Printf("out:\n%s\nerr:\n%s\n", outStr, errStr)
						fmt.Printf("err is: %s\n", err)
						return err
					}

					if !*worker {
						fmt.Printf("starting rke2-server\n")
						cmd := exec.Command("systemctl", "enable", "--now", "rke2-server")
						cmd.Stdout = &stdout
						cmd.Stderr = &stderr
						err := cmd.Run()
						if err != nil {
							outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
							fmt.Printf("out:\n%s\nerr:\n%s\n", outStr, errStr)
							fmt.Printf("err is: %s\n", err)
							return err
						}

						fmt.Printf("/etc/profile.d/rke2.sh\n")
						// Write out a profile.d file to add to $PATH
						err = ioutil.WriteFile("/etc/profile.d/rke2.sh", []byte("PATH=$PATH:/var/lib/rancher/rke2/bin"), 0400)
						if err != nil {
							return err
						}

						fmt.Printf("/root/.kube/config symlink\n")
						kubeConfig := "/root/.kube/config"
						rke2Dest := "/etc/rancher/rke2/rke2.yaml"
						err = os.MkdirAll(filepath.Dir(kubeConfig), 0755)
						if err != nil {
							return err
						}
						err = os.Symlink(rke2Dest, kubeConfig)
						if err != nil {
							return err
						}

						// Use the kubeconfig and the kube api to wait for node to become Ready
						config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
						if err != nil {
							//							fmt.Printf("Error building kubeconfig: %v\n", err)
							return err
						}

						// Create the Kubernetes client
						clientset, err := kubernetes.NewForConfig(config)
						if err != nil {
							//							fmt.Printf("Error creating Kubernetes client: %v\n", err)
							return err
						}

						ready := false

						hostname, err := os.Hostname()
						if err != nil {

							return err
						}

						cnt := 0
						ncnt := 0
						for ok := true; ok; ok = !ready {
							// Fetch the list of nodes
							nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
							if err != nil {
								cnt = 0
								fmt.Printf("node list failed %d\n", ncnt)
								time.Sleep(1 * time.Second)
							} else {
								ncnt = 0
								// Ignore errors silently
								// Iterate through the nodes until this node == Ready before exiting
								for _, node := range nodes.Items {
									if hostname == node.Name {
										for _, condition := range node.Status.Conditions {
											if condition.Type == "Ready" {
												if condition.Status == "True" {
													fmt.Printf("ready\n")
													ready = true
												} else {
													cnt = cnt + 1
													fmt.Printf("not ready: %d\n", cnt)
												}
											}
										}
									}
								}
								// check every second
								time.Sleep(1 * time.Second)
							}
						}
					} else {
						fmt.Printf("starting rke2-agent\n")
						cmd := exec.Command("systemctl", "enable", "--now", "rke2-agent")
						cmd.Stdout = &stdout
						cmd.Stderr = &stderr
						err := cmd.Run()
						if err != nil {
							outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
							fmt.Printf("out:\n%s\nerr:\n%s\n", outStr, errStr)
							fmt.Printf("err is: %s\n", err)
							return err
						}
					}
				}
			}
			os.RemoveAll("/tmp/rke2")
		default:
			fmt.Printf("you should not be running this command generally\n")
		}
	} else {
		fmt.Printf("I am not intended to be used directly outside of tooling\n")
	}
	return nil
}
