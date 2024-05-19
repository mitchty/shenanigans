package cloudinit

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

// growpart:
//
//	mode: auto
//	devices: ['/']
//	ignore_growroot_disabled: false
//
// Just implement what I need for now don't try to boil the dam ocean.
type CloudInitUserData struct {
	FinalMessage string              `yaml:"final_message"`
	Hostname     string              `yaml:"hostname"`
	Fqdn         string              `yaml:"fqdn"`
	SshPwauth    bool                `yaml:"ssh_pwauth"`
	Output       CloudInitUserOutput `yaml:"output"`
	Chpasswd     CloudInitChPasswd   `yaml:"chpasswd"`
	WriteFiles   []CloudInitFileData `yaml:"write_files"`
	Runcmd       [][]string          `yaml:"runcmd"`
	GrowPart     CloudInitGrowPart   `yaml:"growpart"`
}

type CloudInitGrowPart struct {
	Mode                   string   `yaml:"mode"`
	Devices                []string `yaml:"devices"`
	IgnoreGrowRootDisabled bool     `yaml:"ignore_growroot_disabled"`
}

type CloudInitUserOutput struct {
	All string `yaml:"all"`
}

type CloudInitFileData struct {
	Path        string `yaml:"path"`
	Owner       string `yaml:"owner"`
	Permissions string `yaml:"permissions"`
	Content     string `yaml:"content"`
}

type CloudInitChPasswd struct {
	Expire bool                  `yaml:"expire"`
	Users  []CloudInitUserConfig `yaml:"users"`
}

type CloudInitUserConfig struct {
	Name     string `yaml:"name"`
	Password string `yaml:"password"`
	Usertype string `yaml:"type"` // TODOshould be an enum at some point
}

// REJECT TEMPLATES EMBRACE DATA+CODE DOWN WITH THE SILLY INTERMEDIATES
//
// I am not a fan of templates.

// Also todo: make this setup a bit less derp, right now this is in the "make it work" phase of:
// make it work, make it right, make it fast
//
// For us, the second category is where this will end. Maybe adopt the juju approach.
// Return the cloudinit user data we want
func UserData(hostname string, authorized_key string) ([]byte, error) {
	output := CloudInitUserData{
		FinalMessage: "fin",
		Hostname:     hostname,
		Fqdn:         "dev.home.arpa",
		SshPwauth:    true,
		Output: CloudInitUserOutput{
			All: "| tee -a /var/log/cloud-init-output.log",
		},
		Chpasswd: CloudInitChPasswd{
			Expire: false,
			Users: []CloudInitUserConfig{
				CloudInitUserConfig{
					Name:     "root",
					Password: "changeme",
					Usertype: "text",
				},
			},
		},
		WriteFiles: []CloudInitFileData{
			CloudInitFileData{
				Path:        "/root/.ssh/authorized_keys",
				Owner:       "root:root",
				Permissions: "0400",
				Content:     authorized_key,
			},
			CloudInitFileData{
				Path:        "/etc/ssh/sshd_config.d/90-enable-root-login.conf",
				Owner:       "root:root",
				Permissions: "0400",
				Content:     "PermitRootLogin yes\n",
			},
		},
		Runcmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "restart", "--no-block", "sshd.service"},
		},
		GrowPart: CloudInitGrowPart{
			Mode:                   "auto",
			Devices:                []string{"/"},
			IgnoreGrowRootDisabled: true,
		},
	}
	yaml, err := yaml.Marshal(&output)

	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf("#cloud-config\n%s", yaml)), nil
}

type CloudInitMetaData struct {
	Network CloudInitNetwork `yaml:"network"`
	//	WriteFiles []CloudInitFileData `yaml:"write_files"`
	//	Runcmd     [][]string          `yaml:"runcmd"`
}

type CloudInitNetwork struct {
	Version   int                `yaml:"version"`
	Ethernets CloudInitEthernets `yaml:"ethernets"`
}

type CloudInitEthernets struct {
	Eth0 CloudInitDhcp `yaml:"eth0"`
}

type CloudInitDhcp struct {
	Dhcpv4 bool `yaml:"dhcp4"`
	//	Dhcpv6 bool `yaml:"dhcp6"`
}

// Yeah... sue me, its for future when it'll accept args or who knows.
func MetaData() []byte {
	output := CloudInitMetaData{
		Network: CloudInitNetwork{
			Version: 2,
			Ethernets: CloudInitEthernets{
				Eth0: CloudInitDhcp{
					Dhcpv4: true,
					//					Dhcpv6: false,
				},
			},
		},
	}
	yaml, err := yaml.Marshal(&output)

	if err != nil {
		return nil
	}

	return yaml
}
