package interfaces

import "fmt"

// Sender defines the interface for outbound message delivery
// Each interface (CLI, Telegram, etc.) implements this to enable cross-interface messaging
type Sender interface {
	// Send delivers a message to the specified platform ID
	Send(platformID, content string) error

	// InterfaceName returns the interface identifier (e.g., "cli", "telegram")
	InterfaceName() string
}

// OutboundDispatcher manages message delivery across multiple interfaces
type OutboundDispatcher struct {
	senders map[string]Sender
}

// NewOutboundDispatcher creates a new outbound dispatcher
func NewOutboundDispatcher() *OutboundDispatcher {
	return &OutboundDispatcher{
		senders: make(map[string]Sender),
	}
}

// Register adds an interface sender to the dispatcher
// Accepts interface{} to satisfy port contract, type-asserts to Sender internally
func (d *OutboundDispatcher) Register(sender interface{}) {
	s, ok := sender.(Sender)
	if !ok {
		// Panic is acceptable here - this is a programming error, not runtime
		panic(fmt.Sprintf("dispatcher.Register: expected Sender, got %T", sender))
	}
	d.senders[s.InterfaceName()] = s
}

// Send delivers a message to the specified interface and platform ID
// Returns error if interface is not registered
func (d *OutboundDispatcher) Send(iface, platformID, content string) error {
	sender, ok := d.senders[iface]
	if !ok {
		return fmt.Errorf("interface %q not registered", iface)
	}
	return sender.Send(platformID, content)
}
