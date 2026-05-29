package channel

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

type captureInboundProcessor struct {
	got chan capturedInbound
}

type capturedInbound struct {
	cfg ChannelConfig
	msg InboundMessage
}

func (p *captureInboundProcessor) HandleInbound(_ context.Context, cfg ChannelConfig, msg InboundMessage, _ StreamReplySender) error {
	p.got <- capturedInbound{cfg: cfg, msg: msg}
	return nil
}

func TestDispatchOutboundBotMentionsFeishuGroupMention(t *testing.T) {
	t.Parallel()

	sourceCfg := ChannelConfig{
		ID:               "cfg-peiqi",
		BotID:            "bot-peiqi",
		ChannelType:      ChannelTypeFeishu,
		ExternalIdentity: "ou_peiqi",
		SelfIdentity: map[string]any{
			"name":    "PeiqiBot",
			"open_id": "ou_peiqi",
		},
	}
	targetCfg := ChannelConfig{
		ID:               "cfg-george",
		BotID:            "bot-george",
		ChannelType:      ChannelTypeFeishu,
		ExternalIdentity: "ou_george",
		SelfIdentity: map[string]any{
			"name":    "GeorgeBot",
			"open_id": "ou_george",
		},
	}

	processor := &captureInboundProcessor{got: make(chan capturedInbound, 1)}
	manager := NewManager(
		slog.New(slog.DiscardHandler),
		NewRegistry(),
		&fakeConfigStore{configsByType: map[ChannelType][]ChannelConfig{
			ChannelTypeFeishu: {sourceCfg, targetCfg},
		}},
		processor,
	)

	manager.dispatchOutboundBotMentions(context.Background(), sourceCfg, "chat_id:oc_group", Message{
		Text: "@GeorgeBot wake up and work",
	})

	select {
	case got := <-processor.got:
		if got.cfg.BotID != targetCfg.BotID {
			t.Fatalf("target cfg bot = %q, want %q", got.cfg.BotID, targetCfg.BotID)
		}
		if got.msg.BotID != targetCfg.BotID {
			t.Fatalf("inbound bot = %q, want %q", got.msg.BotID, targetCfg.BotID)
		}
		if got.msg.ReplyTarget != "chat_id:oc_group" {
			t.Fatalf("reply target = %q", got.msg.ReplyTarget)
		}
		if got.msg.Conversation.ID != "oc_group" || got.msg.Conversation.Type != ConversationTypeGroup {
			t.Fatalf("conversation = %#v", got.msg.Conversation)
		}
		if got.msg.Message.Text != "@GeorgeBot wake up and work" {
			t.Fatalf("message text = %q", got.msg.Message.Text)
		}
		if got.msg.Message.ID != "" {
			t.Fatalf("synthetic inbound should not use a platform message id, got %q", got.msg.Message.ID)
		}
		if got.msg.Sender.SubjectID != "ou_peiqi" {
			t.Fatalf("sender subject = %q", got.msg.Sender.SubjectID)
		}
		if mentioned, _ := got.msg.Metadata["is_mentioned"].(bool); !mentioned {
			t.Fatalf("expected synthetic message to be marked mentioned")
		}
		if botToBot, _ := got.msg.Metadata["is_bot_to_bot"].(bool); !botToBot {
			t.Fatalf("expected synthetic message to be marked bot-to-bot")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for synthetic inbound")
	}
}

func TestDispatchOutboundBotMentionsSkipsUndirectedText(t *testing.T) {
	t.Parallel()

	sourceCfg := ChannelConfig{
		ID:          "cfg-peiqi",
		BotID:       "bot-peiqi",
		ChannelType: ChannelTypeFeishu,
		SelfIdentity: map[string]any{
			"name": "PeiqiBot",
		},
	}
	targetCfg := ChannelConfig{
		ID:          "cfg-george",
		BotID:       "bot-george",
		ChannelType: ChannelTypeFeishu,
		SelfIdentity: map[string]any{
			"name": "GeorgeBot",
		},
	}
	processor := &captureInboundProcessor{got: make(chan capturedInbound, 1)}
	manager := NewManager(
		slog.New(slog.DiscardHandler),
		NewRegistry(),
		&fakeConfigStore{configsByType: map[ChannelType][]ChannelConfig{
			ChannelTypeFeishu: {sourceCfg, targetCfg},
		}},
		processor,
	)

	manager.dispatchOutboundBotMentions(context.Background(), sourceCfg, "chat_id:oc_group", Message{
		Text: "GeorgeBot is quiet today",
	})

	select {
	case got := <-processor.got:
		t.Fatalf("unexpected synthetic inbound: %#v", got)
	case <-time.After(150 * time.Millisecond):
	}
}
