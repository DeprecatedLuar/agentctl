package gateways

import "fmt"

// Sender defines the interface for outbound message delivery
// Each gateway (CLI, Telegram, etc.) implements this to enable cross-gateway messaging
type Sender interface {
	// Send delivers a message to the specified platform ID
	Send(platformID, content string) error

	// SendToolReport delivers a tool-use report to the specified platform ID.
	// family is the tool's folder-derived family name ("" if the tool sits
	// directly under tools/). Each gateway decides its own rendering (e.g.
	// Telegram shows "[family] message" in a code block, CLI "family: message")
	// instead of receiving a pre-formatted string.
	SendToolReport(platformID, family, message string) error

	// GatewayName returns the gateway identifier (e.g., "cli", "telegram")
	GatewayName() string
}

// OutboundDispatcher manages message delivery across multiple gateways
type OutboundDispatcher struct {
	senders map[string]Sender
}

// NewOutboundDispatcher creates a new outbound dispatcher
func NewOutboundDispatcher() *OutboundDispatcher {
	return &OutboundDispatcher{
		senders: make(map[string]Sender),
	}
}

// Register adds a gateway sender to the dispatcher
func (d *OutboundDispatcher) Register(sender Sender) {
	d.senders[sender.GatewayName()] = sender
}

// Send delivers a message to the specified gateway and platform ID
// Returns error if the gateway is not registered
func (d *OutboundDispatcher) Send(gateway, platformID, content string) error {
	sender, ok := d.senders[gateway]
	if !ok {
		return fmt.Errorf("gateway %q not registered", gateway)
	}
	return sender.Send(platformID, content)
}

// SendToolReport delivers a tool-use report to the specified gateway and
// platform ID. Returns error if the gateway is not registered.
func (d *OutboundDispatcher) SendToolReport(gateway, platformID, family, message string) error {
	sender, ok := d.senders[gateway]
	if !ok {
		return fmt.Errorf("gateway %q not registered", gateway)
	}
	return sender.SendToolReport(platformID, family, message)
}
