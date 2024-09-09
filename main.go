package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"gosshpuppet/internal"
	"gosshpuppet/internal/callback"
	"gosshpuppet/internal/command"
	"gosshpuppet/internal/config"
	"gosshpuppet/internal/handler/directtcpip"
	"gosshpuppet/internal/handler/tcpipforward"
	"gosshpuppet/internal/logging"
	"gosshpuppet/internal/puppet"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

func main() {
	var (
		argListenAddr   string
		argAccessConfig string

		argHostPrivateKeys   StringSliceArg
		argHostSocketNetwork string

		argIdleTimeout    time.Duration
		argOverallTimeout time.Duration

		argDebug   bool
		argVersion bool
	)
	{
		flag.StringVar(&argListenAddr, "listen", ":2222", "Listen address/port")
		flag.StringVar(&argAccessConfig, "access", "./access.yaml", "Access config file")

		flag.Var(&argHostPrivateKeys, "private", "Host private key file, repeatable")
		flag.StringVar(&argHostSocketNetwork, "socket-network", "tcp", "Reverse tunnel socket network")

		flag.DurationVar(&argIdleTimeout, "idle-timeout", time.Minute*3, "Idle session timeout")
		flag.DurationVar(&argOverallTimeout, "overall-timeout", 0, "Overall session timeout")

		flag.BoolVar(&argDebug, "debug", false, "Debug logs")
		flag.BoolVar(&argVersion, "version", false, "Print version and exit")
	}

	flag.Usage = func() {
		w := os.Stdout
		flag.CommandLine.SetOutput(w)
		fmt.Fprintln(w, "Usage:")
		flag.PrintDefaults()
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Signals:")
		fmt.Fprintln(w, "  SIGHUP - Reload access config")
	}
	flag.Parse()

	if argVersion {
		fmt.Println(internal.Version, runtime.Version())
		os.Exit(0)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Logger
	{
		level := slog.LevelInfo
		if argDebug {
			level = slog.LevelDebug
		}
		ctx = logging.NewContext(ctx, logging.NewLogger(level))
	}

	if len(argHostPrivateKeys) == 0 {
		logging.FromContext(ctx).Error("At least one host private key file must be provided")
		os.Exit(1)
	}

	// Load private keys
	var hostSigners []ssh.Signer
	{
		for _, keyFile := range argHostPrivateKeys {
			b, err := os.ReadFile(keyFile)
			if err != nil {
				logging.FromContext(ctx).Error("Failed to read private key file "+keyFile, "err", err)
				os.Exit(1)
			}

			sig, err := gossh.ParsePrivateKey(b)
			if err != nil {
				logging.FromContext(ctx).Error("Failed to parse private key file "+keyFile, "err", err)
				os.Exit(1)
			}

			hostSigners = append(hostSigners, sig)

			logging.FromContext(ctx).Info("Loaded private key file "+keyFile, "type", sig.PublicKey().Type())
		}
	}

	// Config
	var accessConfig = &config.AccessConfigHolder{}
	{
		f, err := os.OpenFile(argAccessConfig, os.O_RDONLY, 0)
		if err != nil {
			logging.FromContext(ctx).Error("Failed to open access config", "err", err)
			os.Exit(1)
		}

		v, err := config.ParseAccessConfig(ctx, f)
		f.Close()
		if err != nil {
			logging.FromContext(ctx).Error("Failed to parse access config", "err", err)
			os.Exit(1)
		}

		accessConfig.Store(v)

		// SIGHUP -> reload access config
		go func(ctx context.Context) {
			ch := make(chan os.Signal, 1)
			defer close(ch)
			signal.Notify(ch, syscall.SIGHUP)
			for {
				select {
				case <-ctx.Done():
					return
				case sig := <-ch:
					logging.FromContext(ctx).Info("Reloading access config by " + sig.String())

					f, err := os.OpenFile(argAccessConfig, os.O_RDONLY, 0)
					if err != nil {
						logging.FromContext(ctx).Error("Failed to open access config", "err", err)
						continue
					}

					v, err := config.ParseAccessConfig(ctx, f)
					f.Close()
					if err != nil {
						logging.FromContext(ctx).Error("Failed to parse access config", "err", err)
						continue
					}

					accessConfig.Store(v)
				}
			}
		}(logging.NewContextGroupWith(ctx, "sighup"))
	}

	puppetManager := puppet.NewMapper()

	// Handlers
	tcpipForwarder, err := tcpipforward.NewTcpipForward(argHostSocketNetwork, puppetManager)
	if err != nil {
		logging.FromContext(ctx).Error("Failed to create tcpip forwarder", "err", err)
		os.Exit(1)
	}
	defer tcpipForwarder.Close()

	tcpipDirecter := directtcpip.NewDirectTcpip(puppetManager)

	// Server
	srv := &ssh.Server{
		Addr:        argListenAddr,
		HostSigners: hostSigners,
		IdleTimeout: argIdleTimeout,
		MaxTimeout:  argOverallTimeout,

		ChannelHandlers: map[string]ssh.ChannelHandler{
			"session":      ssh.DefaultSessionHandler,
			"direct-tcpip": tcpipDirecter.Handle,
		},

		RequestHandlers: map[string]ssh.RequestHandler{
			tcpipforward.ForwardRequestType:       tcpipForwarder.Handle,
			tcpipforward.CancelForwardRequestType: tcpipForwarder.Handle,
		},

		// Connection errors
		ConnectionFailedCallback: func(conn net.Conn, err error) {
			logging.FromContext(ctx).WithGroup("connerrcbk").With("remote", conn.RemoteAddr().String()).Debug("Connection error", "err", err)
		},

		// Public key auth
		PublicKeyHandler: callback.PublicKeyHandler(ctx, accessConfig),

		// Shell/exec session request
		SessionRequestCallback: callback.SessionRequestCallback(ctx),

		// Shell/exec handler (admins)
		Handler: callback.SessionExecCallback(
			command.AdminInterpreter(puppetManager, accessConfig),
		),

		// Reverse/remote port requests (puppets)
		ReversePortForwardingCallback: callback.ReversePortForwardingCallback(ctx, accessConfig),

		// Local port requests (admins)
		LocalPortForwardingCallback: callback.LocalPortForwardingCallback(ctx, accessConfig),
	}

	// Start server
	go func() {
		defer cancel()

		logger := logging.FromContext(ctx).WithGroup("gossh")
		logger.Info("Starting server on " + srv.Addr)

		if err := srv.ListenAndServe(); err != nil {
			if !errors.Is(err, ssh.ErrServerClosed) {
				logger.Error("Failed to start server", "err", err)
			}
		}
	}()

	<-ctx.Done()
	logging.FromContext(ctx).Info("Shutting down server")

	if err := srv.Close(); err != nil {
		logging.FromContext(ctx).Error("Error closing server", "err", err)
	}
}

type StringSliceArg []string

func (s *StringSliceArg) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *StringSliceArg) Set(value string) error {
	*s = append(*s, value)
	return nil
}
