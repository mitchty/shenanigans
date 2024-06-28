package lib

import (
	"embed"
	"fmt"
)

// TODOsplit this into disparate packages maybe?
var EmbedFs embed.FS

func DumpKubeVipYaml() {
	data, _ := EmbedFs.ReadFile("embed/kube-vip-arp.yaml")
	fmt.Printf("debug: %s", data)
}

func KubeVipArpData() []byte {
	data, _ := EmbedFs.ReadFile("embed/kube-vip-arp.yaml")
	return data
}
