package group

// Group record data
type Group struct {
	After  []string
	Config []VmConfig
	Kind   string
	Name   string
	//	Provider string
}

// Generic vm configuration, we need at the least the image to boot
// from, how big /dev/{s,v}da is, how big memory should be, and cpu count
type VmConfig struct {
	Count    int // I'm not overly keen on this being here tbh
	Cpu      int
	Disksize int
	Memory   int
	Name     string
	Qcow2    string
}
