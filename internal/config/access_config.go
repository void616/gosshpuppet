package config

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sync/atomic"

	"github.com/gliderlabs/ssh"
	"gopkg.in/yaml.v3"
)

var DefaultServicePorts = map[uint32]string{
	22: "ssh",
}

type AccessConfigHolder struct {
	atomic.Pointer[AccessConfig]
}

type AccessConfig struct {
	Admins   map[string][]PublicKey `yaml:"admins"`
	Puppets  []*Puppet              `yaml:"puppets"`
	Services map[uint32]string      `yaml:"services"`
}

type Puppet struct {
	Regexp string      `yaml:"regexp"` // regexp to match puppet name (lowercase!)
	Keys   []PublicKey `yaml:"keys"`

	re *regexp.Regexp
}

func ParseAccessConfig(_ context.Context, r io.Reader) (*AccessConfig, error) {
	var c AccessConfig
	if err := yaml.NewDecoder(r).Decode(&c); err != nil {
		return nil, fmt.Errorf("decoding yaml: %w", err)
	}

	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	for _, v := range c.Puppets {
		r, err := regexp.Compile(v.Regexp)
		if err != nil {
			return nil, fmt.Errorf("compiling puppet regexp %q: %w", v.Regexp, err)
		}
		v.re = r
	}

	return &c, nil
}

func (c *AccessConfig) Validate() error {
	if len(c.Admins) == 0 {
		return fmt.Errorf("no admins defined")
	}

	if len(c.Services) == 0 {
		m := make(map[uint32]string)
		for port, name := range DefaultServicePorts {
			m[port] = name
		}
		c.Services = m
	}

	return nil
}

func (c *AccessConfig) IsAdmin(name string, key ssh.PublicKey) bool {
	keys, ok := c.Admins[name]
	if !ok {
		return false
	}

	for _, k := range keys {
		if ssh.KeysEqual(k, key) {
			return true
		}
	}

	return false
}

func (c *AccessConfig) IsPuppet(name string, key ssh.PublicKey) bool {
	for _, pu := range c.Puppets {
		if !pu.re.MatchString(name) {
			continue
		}

		for _, k := range pu.Keys {
			if ssh.KeysEqual(k, key) {
				return true
			}
		}
	}

	return false
}
