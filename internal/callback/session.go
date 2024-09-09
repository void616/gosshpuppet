package callback

import (
	"context"
	"fmt"
	"gosshpuppet/internal/client"
	"gosshpuppet/internal/logging"
	"io"

	"github.com/gliderlabs/ssh"
)

func SessionRequestCallback(baseCtx context.Context) func(ssh.Session, string) bool {
	return func(sess ssh.Session, requestType string) bool {
		ctx := sess.Context()

		logging.FromContext(baseCtx).WithGroup("sesscbk").With(
			"user", ctx.User(),
			"remote", ctx.RemoteAddr().String(),
			"session", ctx.SessionID(),
		).Debug(fmt.Sprintf("Requested session type: %s", requestType))

		cli := client.FromSSHContext(ctx)

		// Shell/exec is only for admins
		if !cli.IsAdmin() {
			sess.Write([]byte("Nope, only for admins.\n"))
			return false
		}

		if requestType != "exec" {
			sess.Write([]byte("Only exec session type is allowed\n"))
			return false
		}

		return true
	}
}

type CommandInterpreter func(ctx context.Context, user string, args []string, w io.Writer) error

func SessionExecCallback(i CommandInterpreter) func(s ssh.Session) {
	return func(sess ssh.Session) {
		cli := client.FromSSHContext(sess.Context())

		if !cli.IsAdmin() {
			sess.Write([]byte("Nope, only for admins.\n"))
			sess.Exit(1)
			return
		}

		args := sess.Command()

		if err := i(sess.Context(), sess.User(), args, sess); err != nil {
			sess.Write([]byte(fmt.Sprintf("Error: %s\n", err)))
			sess.Exit(1)
			return
		}
	}
}
