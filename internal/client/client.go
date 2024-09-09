//go:generate stringer -type=ClientKind -linecomment

package client

import (
	"fmt"
	"log/slog"
	"time"
)

const ClientSSHContextKey = "gosshpuppet-client-kind"

type ClientKind int

const (
	ClientUnknown ClientKind = iota // unknown
	ClientPuppet                    // puppet
	ClientAdmin                     // admin
)

type Client struct {
	name      string
	remote    string
	sessionID string
	kind      ClientKind
	logger    *slog.Logger
	createdAt time.Time
}

func NewClient(name, remote, sessionID string, t ClientKind, logger *slog.Logger, createdAt time.Time) *Client {
	return &Client{
		name:      name,
		remote:    remote,
		sessionID: sessionID,
		kind:      t,
		logger:    logger,
		createdAt: createdAt,
	}
}

func (c *Client) String() string {
	return fmt.Sprintf("%s '%s'", c.kind, c.name)
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) Remote() string {
	return c.remote
}

func (c *Client) SessionID() string {
	return c.sessionID
}

func (c *Client) Kind() ClientKind {
	return c.kind
}

func (c *Client) IsAdmin() bool {
	return c.kind == ClientAdmin
}

func (c *Client) IsPuppet() bool {
	return c.kind == ClientPuppet
}

func (c *Client) Logger() *slog.Logger {
	return c.logger
}

func (c *Client) CreatedAt() time.Time {
	return c.createdAt
}
