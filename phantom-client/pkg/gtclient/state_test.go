package gtclient

import (
	"phantom-client/pkg/token"
	"testing"

	"pgregory.net/rapid"
)

func TestConnState_String(t *testing.T) {
	tests := []struct {
		state ConnState
		want  string
	}{
		{StateInit, "Init"},
		{StateBootstrapping, "Bootstrapping"},
		{StateConnected, "Connected"},
		{StateDegraded, "Degraded"},
		{StateReconnecting, "Reconnecting"},
		{StateExhausted, "Exhausted"},
		{StateStopped, "Stopped"},
		{ConnState(99), "Unknown"},
		{ConnState(-1), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("ConnState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

// Property: all valid states have non-empty string representation
func TestProperty_ConnStateString(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := ConnState(rapid.IntRange(0, 6).Draw(t, "state"))
		name := s.String()
		if name == "" {
			t.Fatalf("state %d has empty string", s)
		}
		if name == "Unknown" {
			t.Fatalf("valid state %d returned Unknown", s)
		}
	})
}

func TestStateMachine_InitialState(t *testing.T) {
	config := makeTestConfig()
	c := NewGTunnelClient(config)
	defer c.Close()

	if c.State() != StateInit {
		t.Fatalf("expected StateInit, got %s", c.State())
	}
	if c.IsConnected() {
		t.Fatal("should not be connected in StateInit")
	}
}

func TestStateMachine_Transition(t *testing.T) {
	config := makeTestConfig()
	c := NewGTunnelClient(config)
	defer c.Close()

	c.transition(StateConnected, "test")
	if c.State() != StateConnected {
		t.Fatalf("expected StateConnected, got %s", c.State())
	}
	if !c.IsConnected() {
		t.Fatal("should be connected in StateConnected")
	}

	c.transition(StateReconnecting, "test")
	if c.IsConnected() {
		t.Fatal("should not be connected in StateReconnecting")
	}
}

func TestStateMachine_Close(t *testing.T) {
	config := makeTestConfig()
	c := NewGTunnelClient(config)
	c.Close()

	if c.State() != StateStopped {
		t.Fatalf("expected StateStopped after Close, got %s", c.State())
	}
}

func TestReconnect_StoppedState(t *testing.T) {
	config := makeTestConfig()
	c := NewGTunnelClient(config)
	c.Close()

	err := c.Reconnect(nil)
	if err == nil {
		t.Fatal("expected error when reconnecting stopped client")
	}
}

func TestReconnect_AlreadyConnected(t *testing.T) {
	config := makeTestConfig()
	c := NewGTunnelClient(config)
	defer c.Close()

	c.transition(StateConnected, "test")
	err := c.Reconnect(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func makeTestConfig() *token.BootstrapConfig {
	return &token.BootstrapConfig{
		BootstrapPool: []token.GatewayEndpoint{
			{IP: "10.0.0.1", Port: 443, Region: "test"},
		},
		PreSharedKey: make([]byte, 32),
	}
}
