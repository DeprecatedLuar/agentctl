package gateways

import (
	"fmt"
	"testing"
)

// mockSender implements the Sender interface for testing
type mockSender struct {
	name     string
	messages []string
	sendErr  error
}

func (m *mockSender) GatewayName() string {
	return m.name
}

func (m *mockSender) Send(platformID, content string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.messages = append(m.messages, fmt.Sprintf("%s:%s", platformID, content))
	return nil
}

func TestOutboundDispatcher_Register(t *testing.T) {
	dispatcher := NewOutboundDispatcher()

	sender := &mockSender{name: "test"}
	dispatcher.Register(sender)

	// Verify sender was registered
	if dispatcher.senders["test"] != sender {
		t.Error("sender was not registered correctly")
	}
}

func TestOutboundDispatcher_Send_Success(t *testing.T) {
	dispatcher := NewOutboundDispatcher()

	sender := &mockSender{name: "telegram"}
	dispatcher.Register(sender)

	err := dispatcher.Send("telegram", "12345", "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify message was delivered
	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sender.messages))
	}

	expected := "12345:hello world"
	if sender.messages[0] != expected {
		t.Errorf("expected %q, got %q", expected, sender.messages[0])
	}
}

func TestOutboundDispatcher_Send_UnknownGateway(t *testing.T) {
	dispatcher := NewOutboundDispatcher()

	err := dispatcher.Send("unknown", "123", "test")
	if err == nil {
		t.Fatal("expected error for unknown gateway")
	}

	expectedMsg := `gateway "unknown" not registered`
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestOutboundDispatcher_Send_SenderError(t *testing.T) {
	dispatcher := NewOutboundDispatcher()

	senderErr := fmt.Errorf("send failed")
	sender := &mockSender{name: "cli", sendErr: senderErr}
	dispatcher.Register(sender)

	err := dispatcher.Send("cli", "user", "test")
	if err == nil {
		t.Fatal("expected error from sender")
	}

	if err != senderErr {
		t.Errorf("expected error %v, got %v", senderErr, err)
	}
}

func TestOutboundDispatcher_MultipleGateways(t *testing.T) {
	dispatcher := NewOutboundDispatcher()

	telegram := &mockSender{name: "telegram"}
	cli := &mockSender{name: "cli", sendErr: fmt.Errorf("not implemented")}

	dispatcher.Register(telegram)
	dispatcher.Register(cli)

	// Send to telegram should succeed
	err := dispatcher.Send("telegram", "123", "msg1")
	if err != nil {
		t.Fatalf("telegram send failed: %v", err)
	}

	// Send to cli should fail with sender's error
	err = dispatcher.Send("cli", "user", "msg2")
	if err == nil {
		t.Fatal("expected cli send to fail")
	}

	// Verify telegram received its message
	if len(telegram.messages) != 1 {
		t.Errorf("expected telegram to have 1 message, got %d", len(telegram.messages))
	}

	// Verify cli has no messages (error prevented delivery)
	if len(cli.messages) != 0 {
		t.Errorf("expected cli to have 0 messages, got %d", len(cli.messages))
	}
}
