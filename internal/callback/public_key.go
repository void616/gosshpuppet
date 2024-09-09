package callback

import (
	"context"
	"fmt"
	"gosshpuppet/internal/client"
	"gosshpuppet/internal/config"
	"gosshpuppet/internal/logging"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/gliderlabs/ssh"
)

const clientKindIdentifiedContextKey = "gosshpuppet-client-kind-identified"

func PublicKeyHandler(baseCtx context.Context, ac *config.AccessConfigHolder) func(ssh.Context, ssh.PublicKey) bool {
	return func(ctx ssh.Context, key ssh.PublicKey) bool {
		logger := logging.FromContext(baseCtx).WithGroup("pubkeycbk").With(
			"user", ctx.User(),
			"remote", ctx.RemoteAddr().String(),
			"format", key.Type(),
			"fingerprint", gossh.FingerprintSHA256(key),
			"session", ctx.SessionID(),
		)

		// Do not re-identify client
		if ctx.Value(clientKindIdentifiedContextKey) != nil {
			logger.Debug("Client is already identified")
			return false
		}

		// Ensure user is lowercased for sake of comparison
		if ctx.User() != strings.ToLower(ctx.User()) {
			logger.Debug("User name is not lowercase, rejecting")
			return false
		}

		var kind = client.ClientUnknown
		var cfg = ac.Load()

		// Detect client kind
		switch {
		case cfg.IsPuppet(ctx.User(), key):
			logger.Debug("Public key found in puppet list")
			kind = client.ClientPuppet

		case cfg.IsAdmin(ctx.User(), key):
			logger.Debug("Public key found in admin list")
			kind = client.ClientAdmin

		default:
			logger.Debug("Public key is unknown")
			return false
		}

		logger.Info(fmt.Sprintf("%v authenticated as %v", ctx.User(), kind))

		client.SetSSHContext(ctx, client.NewClient(
			ctx.User(),
			ctx.RemoteAddr().String(),
			ctx.SessionID(),
			kind,
			logging.FromContext(baseCtx).WithGroup("client").With(
				"name", ctx.User(),
				"kind", kind.String(),
				"session", ctx.SessionID(),
			),
			time.Now(),
		))

		ctx.SetValue(clientKindIdentifiedContextKey, true)
		return true
	}
}
