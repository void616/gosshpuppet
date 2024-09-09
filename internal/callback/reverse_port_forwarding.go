package callback

import (
	"context"
	"fmt"
	"gosshpuppet/internal/client"
	"gosshpuppet/internal/config"
	"gosshpuppet/internal/logging"

	"github.com/gliderlabs/ssh"
)

func ReversePortForwardingCallback(baseCtx context.Context, ac *config.AccessConfigHolder) func(ssh.Context, string, uint32) bool {
	return func(ctx ssh.Context, remoteBindHost string, remoteBindPort uint32) bool {
		logging.FromContext(baseCtx).WithGroup("revportcbk").With(
			"user", ctx.User(),
			"remote", ctx.RemoteAddr().String(),
			"session", ctx.SessionID(),
		).Debug(fmt.Sprintf("Requested reverse port forwarding to %s:%d", remoteBindHost, remoteBindPort))

		cli := client.FromSSHContext(ctx)

		// allowed only for puppets
		if cli.Kind() != client.ClientPuppet {
			cli.Logger().Debug("Reverse port forwarding not allowed for this client type")
			return false
		}

		if remoteBindHost != "localhost" && remoteBindHost != "" && remoteBindHost != "127.0.0.1" {
			cli.Logger().Debug(fmt.Sprintf("Reverse port forwarding allowed only for localhost, got %q", remoteBindHost))
			return false
		}

		// allowed only for specific ports
		if _, ok := ac.Load().Services[remoteBindPort]; !ok {
			cli.Logger().Debug(fmt.Sprintf("Reverse port forwarding not allowed for port %d", remoteBindPort))
			return false
		}

		return true
	}
}
