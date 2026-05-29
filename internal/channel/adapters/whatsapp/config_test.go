package whatsapp

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/memohai/memoh/internal/channel"
)

func TestNormalizeTarget(t *testing.T) {
	tests := map[string]string{
		"+1 (555) 123-4567":             "15551234567@s.whatsapp.net",
		"wa:15551234567":                "15551234567@s.whatsapp.net",
		"15551234567@s.whatsapp.net":    "15551234567@s.whatsapp.net",
		"120363000000000000@g.us":       "120363000000000000@g.us",
		"https://wa.me/+15551234567":    "15551234567@s.whatsapp.net",
		"whatsapp:120363000000000@g.us": "120363000000000@g.us",
	}
	for input, want := range tests {
		if got := normalizeTarget(input); got != want {
			t.Fatalf("normalizeTarget(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseConfigRequiresStorePath(t *testing.T) {
	if _, err := parseConfig(map[string]any{}); err == nil {
		t.Fatal("expected missing storePath error")
	}
}

func TestParseConfigAcceptsProxy(t *testing.T) {
	cases := map[string]bool{
		"http://proxy:3128":     true,
		"https://proxy:3128":    true,
		"socks5://1.2.3.4:1080": true,
		"":                      true, // empty proxy is allowed
	}
	for proxy, ok := range cases {
		_, err := parseConfig(map[string]any{
			"storePath": "/tmp/x.db",
			"proxy":     proxy,
		})
		if ok && err != nil {
			t.Fatalf("parseConfig proxy=%q expected ok, got err=%v", proxy, err)
		}
	}
}

func TestParseConfigRejectsBogusProxy(t *testing.T) {
	_, err := parseConfig(map[string]any{
		"storePath": "/tmp/x.db",
		"proxy":     "ftp://nope",
	})
	if err == nil {
		t.Fatal("expected unsupported proxy scheme error")
	}
}

func TestBuildUserConfigPrefersJID(t *testing.T) {
	id := channel.Identity{
		SubjectID: "15551234567",
		Attributes: map[string]string{
			"chat_jid": "120363000000000000@g.us",
		},
	}
	out := buildUserConfig(id)
	if out["jid"] != "15551234567@s.whatsapp.net" {
		t.Fatalf("buildUserConfig fell back unexpectedly: %#v", out)
	}
}

func TestMatchBindingByPhoneAttribute(t *testing.T) {
	cfg := map[string]any{"jid": "15551234567@s.whatsapp.net"}
	criteria := channel.BindingCriteria{
		SubjectID: "anything-else",
		Attributes: map[string]string{
			"phone": "+1-555-123-4567",
		},
	}
	if !matchBinding(cfg, criteria) {
		t.Fatal("matchBinding should match by normalized phone")
	}
}

func TestExtractMessageText(t *testing.T) {
	cases := []struct {
		name string
		msg  *waE2E.Message
		want string
	}{
		{"conversation", &waE2E.Message{Conversation: proto.String("hi")}, "hi"},
		{"extended", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("hello")}}, "hello"},
		{"image-caption", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("cap")}}, "cap"},
		{"empty", &waE2E.Message{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractMessageText(tc.msg); strings.TrimSpace(got) != tc.want {
				t.Fatalf("extractMessageText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractReplyRefRequiresStanzaID(t *testing.T) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("reply"),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:    proto.String("ABC123"),
				Participant: proto.String("15551234567@s.whatsapp.net"),
				QuotedMessage: &waE2E.Message{
					Conversation: proto.String("the original"),
				},
			},
		},
	}
	ref := extractReplyRef(msg, types.JID{User: "15551234567", Server: "s.whatsapp.net"})
	if ref == nil {
		t.Fatal("expected reply ref")
	}
	if ref.MessageID != "ABC123" {
		t.Fatalf("MessageID = %q, want ABC123", ref.MessageID)
	}
	if ref.Sender != "15551234567@s.whatsapp.net" {
		t.Fatalf("Sender = %q", ref.Sender)
	}
	if ref.Preview != "the original" {
		t.Fatalf("Preview = %q", ref.Preview)
	}
}

func TestExtractReplyRefIgnoresMissingStanza(t *testing.T) {
	msg := &waE2E.Message{Conversation: proto.String("plain")}
	if extractReplyRef(msg, types.EmptyJID) != nil {
		t.Fatal("expected nil reply ref for plain message")
	}
}

func TestBuildOutboundContextInfo(t *testing.T) {
	chat := types.JID{User: "15551234567", Server: "s.whatsapp.net"}
	ctx := buildOutboundContextInfo(&channel.ReplyRef{
		MessageID: "QUOTE1",
		Sender:    "15551234567@s.whatsapp.net",
		Preview:   "hello",
	}, chat)
	if ctx == nil {
		t.Fatal("expected ContextInfo for non-empty reply")
	}
	if ctx.GetStanzaID() != "QUOTE1" {
		t.Fatalf("StanzaID = %q", ctx.GetStanzaID())
	}
	if ctx.GetQuotedMessage().GetConversation() != "hello" {
		t.Fatalf("preview not embedded: %v", ctx.GetQuotedMessage())
	}
}

func TestBuildOutboundContextInfoNilOnEmpty(t *testing.T) {
	if buildOutboundContextInfo(nil, types.EmptyJID) != nil {
		t.Fatal("nil reply must produce nil ContextInfo")
	}
	empty := &channel.ReplyRef{}
	if buildOutboundContextInfo(empty, types.EmptyJID) != nil {
		t.Fatal("empty reply must produce nil ContextInfo")
	}
}

func TestMapMediaType(t *testing.T) {
	if string(mapMediaType(channel.AttachmentImage)) == "" {
		t.Fatal("image media type should not be empty")
	}
	if mapMediaType(channel.AttachmentVoice) == mapMediaType(channel.AttachmentImage) {
		t.Fatal("voice and image must map to different MediaTypes")
	}
}

func TestFormatPairCode(t *testing.T) {
	cases := map[string]string{
		"ABCDEFGH":   "ABCD-EFGH",
		"abcdefgh":   "ABCD-EFGH",
		"AB CD EFGH": "AB CD EFGH",
		"":           "",
	}
	for input, want := range cases {
		if got := formatPairCode(input); got != want {
			t.Fatalf("formatPairCode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEncodeDecodeDataURL(t *testing.T) {
	url := encodeDataURL("text/plain", []byte("hello"))
	if !strings.HasPrefix(url, "data:text/plain;base64,") {
		t.Fatalf("encodeDataURL produced %q", url)
	}
	data, mime, err := decodeDataURL(url)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("decoded payload = %q", data)
	}
	if mime != "text/plain" {
		t.Fatalf("mime = %q", mime)
	}
}
