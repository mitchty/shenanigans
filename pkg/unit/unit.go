package unit

import (
	"fmt"

	"deploy/pkg/vm"
)

// Unit record data
type UserUnit struct {
	After  []string
	Config []vm.UserConfig
	Kind   string
	Name   string
	//	Provider string //TODO for the future
}

type Unit struct {
	After  []string
	Config []vm.Config
	Kind   string
	Name   string
}

// Custom format function for debugging more simply.
func (u Unit) Format(f fmt.State, r rune) {
	out := fmt.Sprintf("unit %s: ", u.Name)
	if u.Kind != "" {
		out = fmt.Sprintf("%s kind: %s", out, u.Kind)
	}
	if len(u.After) > 0 {
		out = fmt.Sprintf("%s after: %s", out, u.After)
	}
	out = fmt.Sprintf("%s %s", out, u.Config)
	f.Write([]byte(out))
}

func (uu UserUnit) ToUnit() (u Unit, e error) {
	u.After = uu.After
	u.Kind = uu.Kind
	u.Name = uu.Name
	var configs []vm.Config

	for _, uc := range uu.Config {
		c, err := uc.ToConfig()
		if err != nil {
			return u, err
		}
		configs = append(configs, c)
	}
	u.Config = configs
	return u, nil
}
