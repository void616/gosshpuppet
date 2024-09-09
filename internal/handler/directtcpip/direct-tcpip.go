// Modified version of
// https://raw.githubusercontent.com/gliderlabs/ssh/v0.3.7/tcpip.go

package directtcpip

import (
	"fmt"
	"gosshpuppet/internal/client"
	"io"
	"net"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// DirectTcpIPHandler is a handler for direct-tcpip channel requests.
type DirectTcpIPHandler struct {
	puppetFinder PuppetFinder
}

// PuppetFinder is an interface for finding an actual local puppet address.
type PuppetFinder interface {
	PuppetAddress(name string, servicePort uint32) (addr string, network string, ok bool)
}

func NewDirectTcpip(pf PuppetFinder) *DirectTcpIPHandler {
	return &DirectTcpIPHandler{
		puppetFinder: pf,
	}
}

func (h *DirectTcpIPHandler) Handle(srv *ssh.Server, conn *gossh.ServerConn, req gossh.NewChannel, ctx ssh.Context) {
	cli := client.FromSSHContext(ctx)
	logger := cli.Logger().WithGroup("directtcpipcbk")

	var reqData = localForwardChannelData{}
	if err := gossh.Unmarshal(req.ExtraData(), &reqData); err != nil {
		logger.Error("Failed to unmarshal request", "err", err)
		req.Reject(gossh.ConnectionFailed, "Error parsing forward data: "+err.Error())
		return
	}

	if srv.LocalPortForwardingCallback == nil || !srv.LocalPortForwardingCallback(ctx, reqData.DestAddr, reqData.DestPort) {
		req.Reject(gossh.Prohibited, fmt.Sprintf("Port %v forwarding is disallowed", reqData.DestPort))
		return
	}

	// find puppet
	target, targetNetwork, ok := h.puppetFinder.PuppetAddress(reqData.DestAddr, reqData.DestPort)
	if !ok {
		logger.Debug(fmt.Sprintf("Puppet not found for %s:%d", reqData.DestAddr, reqData.DestPort))
		req.Reject(gossh.ConnectionFailed, "Puppet not found or requested port is unavailable")
		return
	}

	logger.Debug(fmt.Sprintf("Forwarding to puppet %s:%d => %s", reqData.DestAddr, reqData.DestPort, target))

	var dialer net.Dialer
	puppetConn, err := dialer.DialContext(ctx, targetNetwork, target)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to dial puppet %v", target), "err", err)
		req.Reject(gossh.ConnectionFailed, "Dialing puppet port: "+err.Error())
		return
	}

	ch, reqs, err := req.Accept()
	if err != nil {
		logger.Error("Failed to accept channel", "err", err)
		puppetConn.Close()
		return
	}

	go gossh.DiscardRequests(reqs)

	go func() {
		defer logger.Debug("Direct-tcpip channel closed")

		defer ch.Close()
		defer puppetConn.Close()
		io.Copy(ch, puppetConn)
	}()

	go func() {
		defer ch.Close()
		defer puppetConn.Close()
		io.Copy(puppetConn, ch)
	}()
}

// direct-tcpip data struct as specified in RFC4254, Section 7.2
type localForwardChannelData struct {
	DestAddr string
	DestPort uint32

	OriginAddr string
	OriginPort uint32
}
