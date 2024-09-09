package client

import (
	"gosshpuppet/internal/logging"
	"time"

	"github.com/gliderlabs/ssh"
)

func SetSSHContext(ctx ssh.Context, cli *Client) {
	ctx.SetValue(ClientSSHContextKey, cli)
}

func FromSSHContext(ctx ssh.Context) *Client {
	if v := ctx.Value(ClientSSHContextKey); v != nil {
		return v.(*Client)
	}
	return NewClient("unknown", "unknown", "unknown", ClientUnknown, logging.NoopLogger(), time.Now())
}
