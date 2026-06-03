package whatsapp

import (
	"context"
	"sync"
	"testing"

	"github.com/ZihengXiong/GenMult/internal/channel"
)

type fakeLifecycle struct {
	mu        sync.Mutex
	upserted  bool
	disabled  *bool
	channelID string
}

func (f *fakeLifecycle) UpsertBotChannelConfig(_ context.Context, botID string, ct channel.ChannelType, req channel.UpsertConfigRequest) (channel.ChannelConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserted = true
	f.disabled = req.Disabled
	f.channelID = string(ct) + ":" + botID
	return channel.ChannelConfig{BotID: botID, ChannelType: ct}, nil
}

func TestHandleLoggedOutDisablesConfig(t *testing.T) {
	a := &WhatsAppAdapter{}
	lc := &fakeLifecycle{}
	a.lifecycle = lc

	a.handleLoggedOut(context.Background(), channel.ChannelConfig{
		ID:    "cfg-1",
		BotID: "bot-1",
	})

	if !lc.upserted {
		t.Fatal("expected lifecycle.UpsertBotChannelConfig to be invoked")
	}
	if lc.disabled == nil || !*lc.disabled {
		t.Fatalf("expected disabled=true, got %v", lc.disabled)
	}
	if lc.channelID != "whatsapp:bot-1" {
		t.Fatalf("unexpected channel id %q", lc.channelID)
	}
}

func TestHandleLoggedOutNoOpWithoutLifecycle(t *testing.T) {
	a := &WhatsAppAdapter{}
	// No panic and no observable side effects expected.
	a.handleLoggedOut(context.Background(), channel.ChannelConfig{ID: "cfg-2", BotID: "bot-2"})
}
