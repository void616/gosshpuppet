package callback

import (
	"context"
	"fmt"
	"gosshpuppet/internal/client"
	"gosshpuppet/internal/config"
	"gosshpuppet/internal/logging"

	"github.com/gliderlabs/ssh"
)

func LocalPortForwardingCallback(baseCtx context.Context, ac *config.AccessConfigHolder) func(ssh.Context, string, uint32) bool {
	return func(ctx ssh.Context, targetAddr string, targetPort uint32) bool {
		logging.FromContext(baseCtx).WithGroup("localportcbk").With(
			"user", ctx.User(),
			"remote", ctx.RemoteAddr().String(),
			"session", ctx.SessionID(),
		).Debug(fmt.Sprintf("Requested local port forwarding to %s:%d", targetAddr, targetPort))

		cli := client.FromSSHContext(ctx)

		// allowed only for admins
		if !cli.IsAdmin() {
			cli.Logger().Debug("Local port forwarding not allowed for this client type")
			return false
		}

		// allowed only for specific ports
		if _, ok := ac.Load().Services[targetPort]; !ok {
			cli.Logger().Debug(fmt.Sprintf("Local port forwarding not allowed for port %d", targetPort))
			return false
		}

		return true
	}
}
