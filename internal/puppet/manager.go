package puppet

import (
	"fmt"
	"gosshpuppet/internal/client"
	"gosshpuppet/internal/logging"
	"sync"
	"time"

	"github.com/gliderlabs/ssh"
)

// Manager keeps track of puppet latest sessions identified by name and service port
type Manager struct {
	m       sync.Mutex
	puppets map[namePort]*PuppetSession
}

type namePort struct {
	Name        string
	ServicePort uint32
}

func (np namePort) String() string {
	return np.Name + ":" + string(np.ServicePort)
}

type PuppetSession struct {
	Name           string
	ServicePort    uint32
	Address        string
	AddressNetwork string

	SessionID string
	CreatedAt time.Time
}

func NewMapper() *Manager {
	return &Manager{
		puppets: make(map[namePort]*PuppetSession),
	}
}

func newNamePort(name string, servicePort uint32) namePort {
	return namePort{
		Name:        name,
		ServicePort: servicePort,
	}
}

func (m *Manager) Puppets() map[string]map[uint32]PuppetSession {
	m.m.Lock()
	defer m.m.Unlock()

	puppets := make(map[string]map[uint32]PuppetSession)

	for np, ps := range m.puppets {
		puppets[np.Name] = make(map[uint32]PuppetSession)
		puppets[np.Name][np.ServicePort] = *ps
	}

	return puppets
}

func (m *Manager) OnForwardBegin(ctx ssh.Context, servicePort uint32, actualAddr, addrNetwork string) {
	cli := client.FromSSHContext(ctx)

	namePort := newNamePort(cli.Name(), servicePort)

	ps := &PuppetSession{
		Name:           cli.Name(),
		ServicePort:    servicePort,
		Address:        actualAddr,
		AddressNetwork: addrNetwork,
		SessionID:      cli.SessionID(),
		CreatedAt:      time.Now(),
	}

	m.m.Lock()

	old := m.puppets[namePort]
	m.puppets[namePort] = ps

	m.m.Unlock()

	if old != nil {
		logging.FromContext(ctx).WithGroup("puppet").Debug(fmt.Sprintf("Replaced puppet session %s: %s => %s", namePort, old.SessionID, ps.SessionID))
	}
}

func (m *Manager) OnForwardEnd(ctx ssh.Context, servicePort uint32, actualAddr, addrNetwork string) {
	cli := client.FromSSHContext(ctx)

	namePort := newNamePort(cli.Name(), servicePort)

	m.m.Lock()

	if ps, ok := m.puppets[namePort]; ok && ps.SessionID == cli.SessionID() {
		delete(m.puppets, namePort)
	}

	m.m.Unlock()
}

func (m *Manager) PuppetAddress(name string, servicePort uint32) (addr, network string, ok bool) {
	namePort := newNamePort(name, servicePort)

	m.m.Lock()
	if ps, ok := m.puppets[namePort]; ok {
		addr = ps.Address
		network = ps.AddressNetwork
	}
	m.m.Unlock()

	return addr, network, addr != ""
}
