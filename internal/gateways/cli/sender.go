package cli

import "fmt"

// Sender implements gateways.Sender for the CLI. It is stateless: platformID
// is ignored since a one-shot chat process has exactly one output (stdout).
type Sender struct{}

// NewSender constructs a CLI Sender.
func NewSender() Sender {
	return Sender{}
}

// GatewayName identifies this sender to the OutboundDispatcher.
func (Sender) GatewayName() string {
	return GatewayName
}

// Send prints content to stdout, formatted the same way as the final chat
// reply, so intermediate narration and the closing answer look identical.
func (Sender) Send(platformID, content string) error {
	fmt.Println(FormatForCLI(content))
	return nil
}

// SendToolReport prints a tool-use report as "family:message" (or just the
// message if family is empty), formatted the same way as any other message.
func (s Sender) SendToolReport(platformID, family, message string) error {
	content := message
	if family != "" {
		content = family + ":" + message
	}
	return s.Send(platformID, content)
}
