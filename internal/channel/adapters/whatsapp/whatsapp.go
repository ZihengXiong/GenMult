package whatsapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite" // revive:disable-line:blank-imports required by sqlc tests and driver registration

	"github.com/ZihengXiong/GenMult/internal/channel"
	"github.com/ZihengXiong/GenMult/internal/config"
)

const (
	defaultTextChunkLimit = 4096
	// WhatsApp limits media to 100MB; we are conservative for memory.
	maxInboundMediaSize = 64 * 1024 * 1024
)

// channelLifecycle is the subset of channel.Lifecycle used to update channel
// config when WhatsApp logs out.
type channelLifecycle interface {
	UpsertBotChannelConfig(ctx context.Context, botID string, channelType channel.ChannelType, req channel.UpsertConfigRequest) (channel.ChannelConfig, error)
}

type WhatsAppAdapter struct {
	logger    *slog.Logger
	dataDir   string
	lifecycle channelLifecycle

	mu      sync.Mutex
	clients map[string]*whatsmeow.Client
}

func NewWhatsAppAdapter(log *slog.Logger, dataRoot string) *WhatsAppAdapter {
	if log == nil {
		log = slog.Default()
	}
	return &WhatsAppAdapter{
		logger:  log.With(slog.String("adapter", "whatsapp")),
		dataDir: defaultDataDir(dataRoot),
		clients: map[string]*whatsmeow.Client{},
	}
}

// SetLifecycle wires a channel lifecycle so the adapter can disable a config
// when WhatsApp pushes a LoggedOut event. Safe to leave nil during tests.
func (a *WhatsAppAdapter) SetLifecycle(l *channel.Lifecycle) {
	if a == nil || l == nil {
		return
	}
	a.lifecycle = l
}

func defaultDataDir(dataRoot string) string {
	root := strings.TrimSpace(dataRoot)
	if root == "" {
		root = config.DefaultDataRoot
	}
	return filepath.Join(root, "channels", "whatsapp")
}

func (*WhatsAppAdapter) Type() channel.ChannelType { return Type }

func (*WhatsAppAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        Type,
		DisplayName: "WhatsApp",
		Capabilities: channel.ChannelCapabilities{
			Text:           true,
			Reply:          true,
			Attachments:    true,
			Media:          true,
			BlockStreaming: true,
			ChatTypes:      []string{channel.ConversationTypePrivate, channel.ConversationTypeGroup},
		},
		OutboundPolicy: channel.OutboundPolicy{
			TextChunkLimit: defaultTextChunkLimit,
			MediaOrder:     channel.OutboundOrderTextFirst,
		},
		ConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"storePath": {
					Type:        channel.FieldString,
					Required:    true,
					Title:       "Session Store Path",
					Description: "Local SQLite session store created by WhatsApp QR login.",
				},
				"sessionJid": {
					Type:  channel.FieldString,
					Title: "Session JID",
				},
				"pushName": {
					Type:  channel.FieldString,
					Title: "Push Name",
				},
				"proxy": {
					Type:        channel.FieldString,
					Title:       "Outbound Proxy",
					Description: "Optional proxy for WhatsApp connectivity. Supports http://, https:// and socks5:// URLs.",
				},
				"clientName": {
					Type:        channel.FieldString,
					Title:       "Client Display Name",
					Description: "Optional display name announced to WhatsApp. Format: \"Browser (OS)\". Defaults to \"Chrome (Linux)\".",
				},
			},
		},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"jid": {Type: channel.FieldString, Required: true, Title: "WhatsApp JID"},
			},
		},
		TargetSpec: channel.TargetSpec{
			Format: "<phone>@s.whatsapp.net | <group>@g.us",
			Hints: []channel.TargetHint{
				{Label: "Phone", Example: "15551234567"},
				{Label: "Contact JID", Example: "15551234567@s.whatsapp.net"},
				{Label: "Group JID", Example: "120363000000000000@g.us"},
			},
		},
	}
}

func (*WhatsAppAdapter) NormalizeConfig(raw map[string]any) (map[string]any, error) {
	return normalizeConfig(raw)
}

func (*WhatsAppAdapter) NormalizeUserConfig(raw map[string]any) (map[string]any, error) {
	return normalizeUserConfig(raw)
}

func (*WhatsAppAdapter) NormalizeTarget(raw string) string { return normalizeTarget(raw) }

func (*WhatsAppAdapter) ResolveTarget(userConfig map[string]any) (string, error) {
	return resolveTarget(userConfig)
}

func (*WhatsAppAdapter) MatchBinding(config map[string]any, criteria channel.BindingCriteria) bool {
	return matchBinding(config, criteria)
}

func (*WhatsAppAdapter) BuildUserConfig(identity channel.Identity) map[string]any {
	return buildUserConfig(identity)
}

func (a *WhatsAppAdapter) Connect(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler) (channel.Connection, error) {
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return nil, err
	}
	container, client, err := a.openClient(ctx, parsed)
	if err != nil {
		return nil, err
	}
	if client.Store.ID == nil {
		_ = container.Close()
		return nil, errors.New("whatsapp session is not logged in; scan QR code first")
	}

	connCtx, cancel := context.WithCancel(ctx)
	handlerID := client.AddEventHandler(func(evt any) {
		a.handleEvent(connCtx, cfg, client, handler, evt)
	})
	if err := client.Connect(); err != nil {
		client.RemoveEventHandler(handlerID)
		cancel()
		_ = container.Close()
		return nil, err
	}
	a.setClient(cfg.ID, client)

	stop := func(context.Context) error {
		cancel()
		client.RemoveEventHandler(handlerID)
		client.Disconnect()
		a.deleteClient(cfg.ID)
		return container.Close()
	}
	return channel.NewConnection(cfg, stop), nil
}

func (a *WhatsAppAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.PreparedOutboundMessage) error {
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	client, cleanup, err := a.clientForSend(ctx, cfg.ID, parsed)
	if err != nil {
		return err
	}
	defer cleanup()

	target := normalizeTarget(msg.Target)
	if target == "" {
		return errors.New("whatsapp target is required")
	}
	jid, err := types.ParseJID(target)
	if err != nil {
		return fmt.Errorf("parse whatsapp target: %w", err)
	}

	contextInfo := buildOutboundContextInfo(msg.Message.Message.Reply, jid)

	text := strings.TrimSpace(msg.Message.Message.PlainText())
	if text != "" {
		var waMsg *waE2E.Message
		if contextInfo != nil {
			waMsg = &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text:        proto.String(text),
					ContextInfo: contextInfo,
				},
			}
		} else {
			waMsg = &waE2E.Message{Conversation: proto.String(text)}
		}
		if _, err := client.SendMessage(ctx, jid, waMsg); err != nil {
			return err
		}
		// Once the text carries the reply context, do not repeat the quote on
		// follow-up media.
		contextInfo = nil
	}

	for _, att := range msg.Message.Attachments {
		if err := a.sendAttachment(ctx, client, jid, att, contextInfo); err != nil {
			return err
		}
	}
	return nil
}

func (a *WhatsAppAdapter) OpenStream(_ context.Context, cfg channel.ChannelConfig, target string, _ channel.StreamOptions) (channel.PreparedOutboundStream, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("whatsapp target is required")
	}
	return &blockStream{adapter: a, cfg: cfg, target: target}, nil
}

func (a *WhatsAppAdapter) handleEvent(ctx context.Context, cfg channel.ChannelConfig, client *whatsmeow.Client, handler channel.InboundHandler, evt any) {
	switch v := evt.(type) {
	case *events.Message:
		msg, ok := a.toInboundMessage(ctx, client, cfg, v)
		if !ok {
			return
		}
		if err := handler(ctx, cfg, msg); err != nil && a.logger != nil {
			a.logger.Error("whatsapp inbound handler failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
	case *events.LoggedOut:
		if a.logger != nil {
			a.logger.Warn("whatsapp logged out",
				slog.String("config_id", cfg.ID),
				slog.String("reason", v.Reason.String()),
			)
		}
		a.handleLoggedOut(ctx, cfg)
	}
}

// handleLoggedOut disables the channel config so the UI surfaces the broken
// session and the router stops attempting reconnects on this credential.
func (a *WhatsAppAdapter) handleLoggedOut(ctx context.Context, cfg channel.ChannelConfig) {
	if a.lifecycle == nil || cfg.BotID == "" {
		return
	}
	disabled := true
	if _, err := a.lifecycle.UpsertBotChannelConfig(ctx, cfg.BotID, Type, channel.UpsertConfigRequest{
		Credentials: cfg.Credentials,
		Disabled:    &disabled,
	}); err != nil && a.logger != nil {
		a.logger.Error("whatsapp disable on logout failed",
			slog.String("config_id", cfg.ID),
			slog.Any("error", err),
		)
	}
}

func (a *WhatsAppAdapter) toInboundMessage(ctx context.Context, client *whatsmeow.Client, cfg channel.ChannelConfig, evt *events.Message) (channel.InboundMessage, bool) {
	if evt == nil || evt.Info.IsFromMe {
		return channel.InboundMessage{}, false
	}
	text := extractMessageText(evt.Message)
	attachments := a.extractInboundAttachments(ctx, client, evt.Message)
	if strings.TrimSpace(text) == "" && len(attachments) == 0 {
		return channel.InboundMessage{}, false
	}
	chat := evt.Info.Chat
	sender := evt.Info.Sender
	if sender.IsEmpty() {
		sender = chat
	}
	convType := channel.ConversationTypePrivate
	if chat.Server == types.GroupServer || evt.Info.IsGroup {
		convType = channel.ConversationTypeGroup
	}
	displayName := strings.TrimSpace(evt.Info.PushName)
	if displayName == "" {
		displayName = sender.User
	}
	mentioned := isSelfMentioned(client, evt.Message)
	replyToBot := isReplyToSelf(client, evt.Message)
	receivedAt := evt.Info.Timestamp
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}
	meta := map[string]any{
		"sender_jid":       sender.String(),
		"chat_jid":         chat.String(),
		"is_mentioned":     mentioned,
		"is_reply_to_bot":  replyToBot,
		"whatsapp_message": evt.Info.ID,
	}
	reply := extractReplyRef(evt.Message, sender)
	return channel.InboundMessage{
		Channel:     Type,
		BotID:       cfg.BotID,
		ReplyTarget: chat.String(),
		RouteKey:    channel.GenerateRoutingKey(Type.String(), cfg.BotID, chat.String(), convType, sender.String()),
		Sender: channel.Identity{
			SubjectID:   sender.String(),
			DisplayName: displayName,
			Attributes: map[string]string{
				"jid":        sender.String(),
				"sender_jid": sender.String(),
				"chat_jid":   chat.String(),
				"phone":      sender.User,
			},
		},
		Conversation: channel.Conversation{
			ID:       chat.String(),
			Type:     convType,
			Name:     chat.User,
			Metadata: map[string]any{"jid": chat.String()},
		},
		Message: channel.Message{
			ID:          evt.Info.ID,
			Format:      channel.MessageFormatPlain,
			Text:        strings.TrimSpace(text),
			Attachments: attachments,
			Reply:       reply,
		},
		ReceivedAt: receivedAt,
		Source:     Type.String(),
		Metadata:   meta,
	}, true
}

func extractMessageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	switch {
	case strings.TrimSpace(msg.GetConversation()) != "":
		return msg.GetConversation()
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetText()
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetCaption()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetCaption()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetCaption()
	default:
		return ""
	}
}

func extractReplyRef(msg *waE2E.Message, fallbackSender types.JID) *channel.ReplyRef {
	ctxInfo := messageContextInfo(msg)
	if ctxInfo == nil {
		return nil
	}
	stanza := strings.TrimSpace(ctxInfo.GetStanzaID())
	if stanza == "" {
		return nil
	}
	sender := strings.TrimSpace(ctxInfo.GetParticipant())
	if sender == "" {
		sender = fallbackSender.String()
	}
	preview := ""
	if quoted := ctxInfo.GetQuotedMessage(); quoted != nil {
		preview = strings.TrimSpace(extractMessageText(quoted))
	}
	return &channel.ReplyRef{
		MessageID: stanza,
		Sender:    sender,
		Preview:   preview,
	}
}

func isSelfMentioned(client *whatsmeow.Client, msg *waE2E.Message) bool {
	if client == nil || client.Store.ID == nil || msg == nil {
		return false
	}
	self := client.Store.ID.ToNonAD().String()
	for _, jid := range mentionedJIDs(msg) {
		if strings.EqualFold(jid, self) {
			return true
		}
	}
	return false
}

func isReplyToSelf(client *whatsmeow.Client, msg *waE2E.Message) bool {
	if client == nil || client.Store.ID == nil || msg == nil {
		return false
	}
	ctxInfo := messageContextInfo(msg)
	if ctxInfo == nil {
		return false
	}
	participant := normalizeTarget(ctxInfo.GetParticipant())
	return participant != "" && strings.EqualFold(participant, client.Store.ID.ToNonAD().String())
}

func mentionedJIDs(msg *waE2E.Message) []string {
	ctxInfo := messageContextInfo(msg)
	if ctxInfo == nil {
		return nil
	}
	return ctxInfo.GetMentionedJID()
}

func messageContextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	if msg == nil {
		return nil
	}
	if msg.GetExtendedTextMessage() != nil {
		return msg.GetExtendedTextMessage().GetContextInfo()
	}
	if msg.GetImageMessage() != nil {
		return msg.GetImageMessage().GetContextInfo()
	}
	if msg.GetVideoMessage() != nil {
		return msg.GetVideoMessage().GetContextInfo()
	}
	if msg.GetDocumentMessage() != nil {
		return msg.GetDocumentMessage().GetContextInfo()
	}
	if msg.GetAudioMessage() != nil {
		return msg.GetAudioMessage().GetContextInfo()
	}
	return nil
}

func (*WhatsAppAdapter) openClient(ctx context.Context, cfg Config) (*sqlstore.Container, *whatsmeow.Client, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.StorePath), 0o700); err != nil {
		return nil, nil, err
	}
	container, err := sqlstore.New(ctx, "sqlite", sqliteStoreDSN(cfg.StorePath), nil)
	if err != nil {
		return nil, nil, err
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, nil, err
	}
	client := whatsmeow.NewClient(device, nil)
	if cfg.Proxy != "" {
		if err := client.SetProxyAddress(cfg.Proxy); err != nil {
			_ = container.Close()
			return nil, nil, fmt.Errorf("apply whatsapp proxy: %w", err)
		}
	}
	return container, client, nil
}

func sqliteStoreDSN(path string) string {
	return filepath.Clean(path) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

func (a *WhatsAppAdapter) defaultStorePath(botID string) string {
	name := strings.TrimSpace(botID)
	if name == "" {
		name = "default"
	}
	return filepath.Join(a.dataDir, name+".db")
}

func (a *WhatsAppAdapter) setClient(configID string, client *whatsmeow.Client) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clients[configID] = client
}

func (a *WhatsAppAdapter) deleteClient(configID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.clients, configID)
}

func (a *WhatsAppAdapter) clientForSend(ctx context.Context, configID string, cfg Config) (*whatsmeow.Client, func(), error) {
	a.mu.Lock()
	client := a.clients[configID]
	a.mu.Unlock()
	if client != nil && client.IsConnected() {
		return client, func() {}, nil
	}
	container, client, err := a.openClient(ctx, cfg)
	if err != nil {
		return nil, func() {}, err
	}
	if client.Store.ID == nil {
		_ = container.Close()
		return nil, func() {}, errors.New("whatsapp session is not logged in")
	}
	if err := client.Connect(); err != nil {
		_ = container.Close()
		return nil, func() {}, err
	}
	cleanup := func() {
		client.Disconnect()
		_ = container.Close()
	}
	return client, cleanup, nil
}

// ---------------------------------------------------------------------------
// Inbound media
// ---------------------------------------------------------------------------

func (a *WhatsAppAdapter) extractInboundAttachments(ctx context.Context, client *whatsmeow.Client, msg *waE2E.Message) []channel.Attachment {
	if msg == nil || client == nil {
		return nil
	}

	switch {
	case msg.GetImageMessage() != nil:
		im := msg.GetImageMessage()
		att := a.downloadAttachment(ctx, client, msg, channel.AttachmentImage,
			im.GetMimetype(), "", im.GetCaption(), int64(im.GetFileLength())) //nolint:gosec // file length fits safely in int64
		att.Width = int(im.GetWidth())
		att.Height = int(im.GetHeight())
		return []channel.Attachment{channel.NormalizeInboundChannelAttachment(att)}

	case msg.GetVideoMessage() != nil:
		vm := msg.GetVideoMessage()
		att := a.downloadAttachment(ctx, client, msg, channel.AttachmentVideo,
			vm.GetMimetype(), "", vm.GetCaption(), int64(vm.GetFileLength())) //nolint:gosec // file length fits safely in int64
		att.Width = int(vm.GetWidth())
		att.Height = int(vm.GetHeight())
		att.DurationMs = int64(vm.GetSeconds()) * 1000
		return []channel.Attachment{channel.NormalizeInboundChannelAttachment(att)}

	case msg.GetAudioMessage() != nil:
		am := msg.GetAudioMessage()
		attType := channel.AttachmentAudio
		if am.GetPTT() {
			attType = channel.AttachmentVoice
		}
		att := a.downloadAttachment(ctx, client, msg, attType,
			am.GetMimetype(), "", "", int64(am.GetFileLength())) //nolint:gosec // file length fits safely in int64
		att.DurationMs = int64(am.GetSeconds()) * 1000
		return []channel.Attachment{channel.NormalizeInboundChannelAttachment(att)}

	case msg.GetDocumentMessage() != nil:
		dm := msg.GetDocumentMessage()
		att := a.downloadAttachment(ctx, client, msg, channel.AttachmentFile,
			dm.GetMimetype(), dm.GetFileName(), dm.GetCaption(), int64(dm.GetFileLength())) //nolint:gosec // file length fits safely in int64
		return []channel.Attachment{channel.NormalizeInboundChannelAttachment(att)}

	case msg.GetStickerMessage() != nil:
		sm := msg.GetStickerMessage()
		att := a.downloadAttachment(ctx, client, msg, channel.AttachmentImage,
			sm.GetMimetype(), "", "", int64(sm.GetFileLength())) //nolint:gosec // file length fits safely in int64
		return []channel.Attachment{channel.NormalizeInboundChannelAttachment(att)}
	}

	return nil
}

func (a *WhatsAppAdapter) downloadAttachment(
	ctx context.Context,
	client *whatsmeow.Client,
	msg *waE2E.Message,
	attType channel.AttachmentType,
	mime, name, caption string,
	declaredSize int64,
) channel.Attachment {
	att := channel.Attachment{
		Type:           attType,
		Mime:           strings.TrimSpace(mime),
		Name:           strings.TrimSpace(name),
		Caption:        strings.TrimSpace(caption),
		Size:           declaredSize,
		SourcePlatform: Type.String(),
	}
	if declaredSize > 0 && declaredSize > maxInboundMediaSize {
		if a.logger != nil {
			a.logger.Warn("whatsapp inbound media exceeds size limit",
				slog.Int64("size", declaredSize),
				slog.Int64("limit", maxInboundMediaSize),
			)
		}
		return att
	}
	//nolint:staticcheck // ignore SA1019: client.DownloadAny is deprecated
	data, err := client.DownloadAny(ctx, msg)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("whatsapp inbound download failed", slog.Any("error", err))
		}
		return att
	}
	if len(data) > maxInboundMediaSize {
		if a.logger != nil {
			a.logger.Warn("whatsapp inbound media truncated", slog.Int("size", len(data)))
		}
		return att
	}
	att.Size = int64(len(data))
	att.Base64 = encodeDataURL(att.Mime, data)
	return att
}

func encodeDataURL(mime string, data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if strings.TrimSpace(mime) == "" {
		mime = http.DetectContentType(data)
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, base64Encode(data))
}

// ---------------------------------------------------------------------------
// Outbound media
// ---------------------------------------------------------------------------

func (*WhatsAppAdapter) sendAttachment(
	ctx context.Context,
	client *whatsmeow.Client,
	jid types.JID,
	att channel.PreparedAttachment,
	contextInfo *waE2E.ContextInfo,
) error {
	data, err := readPreparedAttachment(ctx, att)
	if err != nil {
		return fmt.Errorf("read whatsapp attachment: %w", err)
	}
	mediaType := mapMediaType(att.Logical.Type)
	uploaded, err := client.Upload(ctx, data, mediaType)
	if err != nil {
		return fmt.Errorf("upload whatsapp media: %w", err)
	}
	mime := strings.TrimSpace(att.Mime)
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	caption := strings.TrimSpace(att.Logical.Caption)

	msg := &waE2E.Message{}
	switch att.Logical.Type {
	case channel.AttachmentImage, channel.AttachmentGIF:
		msg.ImageMessage = &waE2E.ImageMessage{
			Caption:       captionPtr(caption),
			Mimetype:      proto.String(mime),
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &uploaded.FileLength,
			ContextInfo:   contextInfo,
		}
	case channel.AttachmentVideo:
		msg.VideoMessage = &waE2E.VideoMessage{
			Caption:       captionPtr(caption),
			Mimetype:      proto.String(mime),
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &uploaded.FileLength,
			ContextInfo:   contextInfo,
		}
	case channel.AttachmentVoice, channel.AttachmentAudio:
		msg.AudioMessage = &waE2E.AudioMessage{
			Mimetype:      proto.String(mime),
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &uploaded.FileLength,
			PTT:           proto.Bool(att.Logical.Type == channel.AttachmentVoice),
			ContextInfo:   contextInfo,
		}
	default:
		fileName := strings.TrimSpace(att.Name)
		if fileName == "" {
			fileName = strings.TrimSpace(att.Logical.Name)
		}
		if fileName == "" {
			fileName = "file"
		}
		msg.DocumentMessage = &waE2E.DocumentMessage{
			Caption:       captionPtr(caption),
			Mimetype:      proto.String(mime),
			FileName:      proto.String(fileName),
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &uploaded.FileLength,
			ContextInfo:   contextInfo,
		}
	}

	_, err = client.SendMessage(ctx, jid, msg)
	return err
}

func mapMediaType(attType channel.AttachmentType) whatsmeow.MediaType {
	switch attType {
	case channel.AttachmentImage, channel.AttachmentGIF:
		return whatsmeow.MediaImage
	case channel.AttachmentVideo:
		return whatsmeow.MediaVideo
	case channel.AttachmentAudio, channel.AttachmentVoice:
		return whatsmeow.MediaAudio
	default:
		return whatsmeow.MediaDocument
	}
}

func captionPtr(caption string) *string {
	if strings.TrimSpace(caption) == "" {
		return nil
	}
	return proto.String(caption)
}

func readPreparedAttachment(ctx context.Context, att channel.PreparedAttachment) ([]byte, error) {
	if att.Open == nil {
		return nil, errors.New("whatsapp attachment is not openable")
	}
	rc, err := att.Open(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(rc, maxInboundMediaSize+1)); err != nil {
		return nil, err
	}
	if buf.Len() > maxInboundMediaSize {
		return nil, fmt.Errorf("whatsapp attachment exceeds %d bytes", maxInboundMediaSize)
	}
	return buf.Bytes(), nil
}

// buildOutboundContextInfo constructs a quoted-message context if the message
// carries a Reply ref. It returns nil when there is no reply or the reply has
// no message id.
func buildOutboundContextInfo(reply *channel.ReplyRef, chat types.JID) *waE2E.ContextInfo {
	if reply == nil || strings.TrimSpace(reply.MessageID) == "" {
		return nil
	}
	participant := strings.TrimSpace(reply.Sender)
	if participant == "" {
		participant = chat.String()
	}
	preview := strings.TrimSpace(reply.Preview)
	quoted := &waE2E.Message{Conversation: proto.String(preview)}
	if preview == "" {
		// WhatsApp requires a non-nil quoted message; placeholder is fine.
		quoted = &waE2E.Message{Conversation: proto.String("")}
	}
	return &waE2E.ContextInfo{
		StanzaID:      proto.String(reply.MessageID),
		Participant:   proto.String(participant),
		QuotedMessage: quoted,
	}
}

// ---------------------------------------------------------------------------
// AttachmentResolver
// ---------------------------------------------------------------------------

// ResolveAttachment satisfies channel.AttachmentResolver, returning an inline
// reader for the (already-decrypted) inbound media that we cached on the
// attachment as a data URL during ingestion.
func (*WhatsAppAdapter) ResolveAttachment(_ context.Context, _ channel.ChannelConfig, attachment channel.Attachment) (channel.AttachmentPayload, error) {
	if strings.TrimSpace(attachment.Base64) == "" {
		return channel.AttachmentPayload{}, errors.New("whatsapp attachment payload not available; download was skipped or failed")
	}
	data, mime, err := decodeDataURL(attachment.Base64)
	if err != nil {
		return channel.AttachmentPayload{}, err
	}
	if strings.TrimSpace(attachment.Mime) != "" {
		mime = attachment.Mime
	}
	return channel.AttachmentPayload{
		Reader: io.NopCloser(bytes.NewReader(data)),
		Mime:   mime,
		Name:   strings.TrimSpace(attachment.Name),
		Size:   int64(len(data)),
	}, nil
}

// ---------------------------------------------------------------------------
// Block stream: buffer deltas and emit a single message at Close
// ---------------------------------------------------------------------------

type blockStream struct {
	adapter *WhatsAppAdapter
	cfg     channel.ChannelConfig
	target  string
	text    strings.Builder
	final   *channel.PreparedMessage
	closed  bool
}

func (s *blockStream) Push(_ context.Context, event channel.PreparedStreamEvent) error {
	if s.closed {
		return nil
	}
	switch event.Type {
	case channel.StreamEventDelta:
		if event.Phase != channel.StreamPhaseReasoning {
			s.text.WriteString(event.Delta)
		}
	case channel.StreamEventFinal:
		if event.Final != nil {
			msg := event.Final.Message
			s.final = &msg
		}
	}
	return nil
}

func (s *blockStream) Close(ctx context.Context) error {
	if s.closed {
		return nil
	}
	s.closed = true
	msg := channel.PreparedMessage{Message: channel.Message{Format: channel.MessageFormatPlain}}
	if s.final != nil {
		msg = *s.final
	}
	if strings.TrimSpace(msg.Message.Text) == "" {
		msg.Message.Text = strings.TrimSpace(s.text.String())
	}
	if msg.Message.IsEmpty() {
		return nil
	}
	return s.adapter.Send(ctx, s.cfg, channel.PreparedOutboundMessage{
		Target:  s.target,
		Message: msg,
	})
}
