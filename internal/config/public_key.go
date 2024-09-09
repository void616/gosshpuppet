package config

import (
	"fmt"

	"github.com/gliderlabs/ssh"
	"gopkg.in/yaml.v3"
)

type PublicKey struct {
	ssh.PublicKey
}

func (k *PublicKey) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("decoding public key: %w", err)
	}

	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(s))
	if err != nil {
		return fmt.Errorf("parsing public key: %w", err)
	}

	k.PublicKey = pk
	return nil
}
