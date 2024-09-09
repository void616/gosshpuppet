// Modified version of
// https://raw.githubusercontent.com/gliderlabs/ssh/v0.3.7/tcpip.go

package tcpipforward

import (
	"fmt"
	"gosshpuppet/internal/client"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
	gossh "golang.org/x/crypto/ssh"
)

const (
	DefaultNetwork = "tcp"

	ForwardRequestType       = "tcpip-forward"
	CancelForwardRequestType = "cancel-tcpip-forward"

	ForwardedTCPChannelType = "forwarded-tcpip"
)

// TcpIpForwardHandler is a handler for reverse port forwarding.
type TcpIpForwardHandler struct {
	m        sync.Mutex
	forwards map[string]map[uint32]net.Listener // session id -> service port -> listener

	network     string
	portManager PortManager

	unixSocketDir string
}

// PortManager is an interface for notifying when a port forwarding starts or ends.
type PortManager interface {
	OnForwardBegin(ctx ssh.Context, servicePort uint32, actualAddr, addrNetwork string)
	OnForwardEnd(ctx ssh.Context, servicePort uint32, actualAddr, addrNetwork string)
}

func NewTcpipForward(network string, pm PortManager) (*TcpIpForwardHandler, error) {
	if network == "" {
		network = DefaultNetwork
	}

	unixSockDir := ""
	if strings.HasPrefix(network, "unix") {
		d, err := os.MkdirTemp("", "gosshpuppet-*")
		if err != nil {
			return nil, fmt.Errorf("making unix socket dir: %w", err)
		}
		unixSockDir = d
	}

	return &TcpIpForwardHandler{
		forwards:      make(map[string]map[uint32]net.Listener),
		network:       network,
		portManager:   pm,
		unixSocketDir: unixSockDir,
	}, nil
}

func (h *TcpIpForwardHandler) Close() error {
	if h.unixSocketDir != "" {
		if err := os.RemoveAll(h.unixSocketDir); err != nil {
			return fmt.Errorf("removing unix socket dir: %w", err)
		}
	}
	return nil
}

func (h *TcpIpForwardHandler) Handle(ctx ssh.Context, srv *ssh.Server, req *gossh.Request) (bool, []byte) {
	cli := client.FromSSHContext(ctx)
	logger := cli.Logger().WithGroup("tcpipforwardcbk").With("request", req.Type)

	conn := ctx.Value(ssh.ContextKeyConn).(*gossh.ServerConn)

	switch req.Type {
	case ForwardRequestType:
		var reqPayload remoteForwardRequest
		if err := gossh.Unmarshal(req.Payload, &reqPayload); err != nil {
			logger.Error("Failed to unmarshal request", "err", err)
			return false, []byte{}
		}

		if srv.ReversePortForwardingCallback == nil || !srv.ReversePortForwardingCallback(ctx, reqPayload.BindAddr, reqPayload.BindPort) {
			return false, []byte("port forwarding is disabled")
		}

		// only one per session
		{
			allocated := false
			h.m.Lock()
			if v, ok := h.forwards[ctx.SessionID()]; ok && v[reqPayload.BindPort] != nil {
				allocated = true
			}
			h.m.Unlock()

			if allocated {
				return false, []byte("this port is already allocated")
			}
		}

		var listenAddr = "localhost:0"

		if strings.HasPrefix(h.network, "unix") {
			listenAddr = filepath.Join(h.unixSocketDir, uuid.NewString()+".sock")
		}

		ln, err := net.Listen(h.network, listenAddr)
		if err != nil {
			logger.Error("Failed to listen", "err", err)
			return false, []byte{}
		}

		boundAddress := ln.Addr().String()
		logger.Debug(fmt.Sprintf("Forwarding listener started on %s", boundAddress))

		var boundPort uint32
		if strings.HasPrefix(h.network, "unix") {
			boundPort = reqPayload.BindPort
		} else {
			_, listenerActualPort, err := net.SplitHostPort(boundAddress)
			if err != nil {
				logger.Error("Failed to split listener address", "err", err)
				ln.Close()
				return false, []byte{}
			}

			v, err := strconv.ParseUint(listenerActualPort, 10, 32)
			if err != nil {
				logger.Error("Failed to convert listener port", "err", err)
				ln.Close()
				return false, []byte{}
			}

			boundPort = uint32(v)
		}

		{
			h.m.Lock()
			if _, ok := h.forwards[ctx.SessionID()]; !ok {
				h.forwards[ctx.SessionID()] = make(map[uint32]net.Listener)
			}
			if _, ok := h.forwards[ctx.SessionID()][reqPayload.BindPort]; ok {
				h.m.Unlock()
				ln.Close()
				return false, []byte("this port is already allocated")
			}

			h.forwards[ctx.SessionID()][reqPayload.BindPort] = ln
			h.m.Unlock()
		}

		go func() {
			logger := logger.WithGroup("canceller")
			defer logger.Debug("Forwarding listener routine end")

			<-ctx.Done()

			h.m.Lock()
			ln := h.removeListener(ctx.SessionID(), reqPayload.BindPort)
			h.m.Unlock()

			if ln != nil {
				ln.Close()
			}
		}()

		// Serve listener
		go func() {
			logger := logger.WithGroup("listener")
			defer logger.Debug("Forwarding listener routine end")

			h.portManager.OnForwardBegin(ctx, reqPayload.BindPort, boundAddress, h.network)
			defer h.portManager.OnForwardEnd(ctx, reqPayload.BindPort, boundAddress, h.network)

			for {
				c, err := ln.Accept()
				if err != nil {
					logger.Debug("Failed to accept connection", "err", err)
					break
				}

				logger.Debug("Accepted connection", "remote", c.RemoteAddr())

				var (
					originAddr = "127.0.0.1"
					originPort = uint32(0)
				)
				if !strings.HasPrefix(h.network, "unix") {
					addr, portStr, err := net.SplitHostPort(c.RemoteAddr().String())
					if err != nil {
						logger.Error("Failed to split remote address", "err", err)
						c.Close()
						continue
					}

					port, err := strconv.ParseUint(portStr, 10, 32)
					if err != nil {
						logger.Error("Failed to parse remote port", "err", err)
						c.Close()
						continue
					}

					originAddr = addr
					originPort = uint32(port)
				}

				payload := gossh.Marshal(&remoteForwardChannelData{
					DestAddr:   reqPayload.BindAddr,
					DestPort:   reqPayload.BindPort,
					OriginAddr: originAddr,
					OriginPort: originPort,
				})

				go func() {
					logger := logger.WithGroup("conn")
					defer logger.Debug("Forwarding connection closed")

					ch, reqs, err := conn.OpenChannel(ForwardedTCPChannelType, payload)
					if err != nil {
						logger.Error("Failed to open channel", "err", err)
						c.Close()
						return
					}

					go gossh.DiscardRequests(reqs)
					go func() {
						defer ch.Close()
						defer c.Close()
						io.Copy(ch, c)
					}()
					go func() {
						defer ch.Close()
						defer c.Close()
						io.Copy(c, ch)
					}()
				}()
			}

			h.m.Lock()
			ln := h.removeListener(ctx.SessionID(), uint32(reqPayload.BindPort))
			h.m.Unlock()

			if ln != nil {
				ln.Close()
			}
		}()

		return true, gossh.Marshal(&remoteForwardSuccess{
			BindPort: boundPort,
		})

	case CancelForwardRequestType:
		var reqPayload remoteForwardCancelRequest
		if err := gossh.Unmarshal(req.Payload, &reqPayload); err != nil {
			logger.Error("Failed to unmarshal request", "err", err)
			return false, []byte{}
		}

		h.m.Lock()
		ln := h.removeListener(ctx.SessionID(), reqPayload.BindPort)
		h.m.Unlock()

		if ln != nil {
			ln.Close()
		}

		return true, nil

	default:
		return false, nil
	}
}

func (h *TcpIpForwardHandler) removeListener(sessionID string, servicePort uint32) net.Listener {
	var ret net.Listener

	if _, ok := h.forwards[sessionID]; ok {
		if ln, ok := h.forwards[sessionID][servicePort]; ok {
			ret = ln
		}

		delete(h.forwards[sessionID], servicePort)
		if len(h.forwards[sessionID]) == 0 {
			delete(h.forwards, sessionID)
		}
	}

	return ret
}

type remoteForwardRequest struct {
	BindAddr string
	BindPort uint32
}

type remoteForwardSuccess struct {
	BindPort uint32
}

type remoteForwardCancelRequest struct {
	BindAddr string
	BindPort uint32
}

type remoteForwardChannelData struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}
