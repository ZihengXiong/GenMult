package channel

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

const botToBotDispatchTimeout = 5 * time.Second

func (m *Manager) dispatchOutboundBotMentions(ctx context.Context, sourceCfg ChannelConfig, target string, msg Message) {
	if m == nil || m.service == nil {
		return
	}
	channelType := sourceCfg.ChannelType
	if channelType != ChannelTypeFeishu {
		return
	}
	target = strings.TrimSpace(target)
	conversationID, ok := groupConversationIDFromTarget(channelType, target)
	if !ok {
		return
	}
	text := strings.TrimSpace(msg.PlainText())
	if text == "" {
		return
	}

	dispatchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), botToBotDispatchTimeout)
	defer cancel()

	configs, err := m.service.ListConfigsByType(dispatchCtx, channelType)
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("bot mention dispatch config lookup failed",
				slog.String("channel", channelType.String()),
				slog.Any("error", err),
			)
		}
		return
	}

	mentioned := mentionedBotConfigs(configs, sourceCfg, msg)
	if len(mentioned) == 0 {
		return
	}

	sender := botSenderIdentity(sourceCfg)
	for _, targetCfg := range mentioned {
		targetRef := botMentionTargetRef(targetCfg)
		metadata := map[string]any{
			"is_mentioned":        true,
			"is_bot_to_bot":       true,
			"synthetic":           true,
			"raw_chat_type":       "group",
			"source_bot_id":       strings.TrimSpace(sourceCfg.BotID),
			"source_config_id":    strings.TrimSpace(sourceCfg.ID),
			"mentioned_targets":   []string{targetRef},
			"mentioned_bot_id":    strings.TrimSpace(targetCfg.BotID),
			"mentioned_config_id": strings.TrimSpace(targetCfg.ID),
		}
		if targetRef == "" {
			delete(metadata, "mentioned_targets")
		}
		inbound := InboundMessage{
			Channel:     channelType,
			BotID:       strings.TrimSpace(targetCfg.BotID),
			ReplyTarget: target,
			Message: Message{
				Text:   text,
				Format: MessageFormatPlain,
			},
			Sender: sender,
			Conversation: Conversation{
				ID:   conversationID,
				Type: ConversationTypeGroup,
			},
			ReceivedAt: time.Now().UTC(),
			Source:     channelType.String(),
			Metadata:   metadata,
		}
		if err := m.HandleInbound(dispatchCtx, targetCfg, inbound); err != nil && m.logger != nil {
			m.logger.Warn("bot mention dispatch failed",
				slog.String("channel", channelType.String()),
				slog.String("source_bot_id", strings.TrimSpace(sourceCfg.BotID)),
				slog.String("target_bot_id", strings.TrimSpace(targetCfg.BotID)),
				slog.Any("error", err),
			)
		}
	}
}

func groupConversationIDFromTarget(channelType ChannelType, target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	switch channelType {
	case ChannelTypeFeishu:
		const prefix = "chat_id:"
		if !strings.HasPrefix(target, prefix) {
			return "", false
		}
		conversationID := strings.TrimSpace(strings.TrimPrefix(target, prefix))
		return conversationID, conversationID != ""
	default:
		return "", false
	}
}

func mentionedBotConfigs(configs []ChannelConfig, sourceCfg ChannelConfig, msg Message) []ChannelConfig {
	if len(configs) == 0 {
		return nil
	}
	text := strings.TrimSpace(msg.PlainText())
	if text == "" && len(msg.Parts) == 0 {
		return nil
	}
	result := make([]ChannelConfig, 0)
	for _, cfg := range configs {
		if cfg.Disabled || strings.TrimSpace(cfg.BotID) == "" {
			continue
		}
		if strings.TrimSpace(cfg.BotID) == strings.TrimSpace(sourceCfg.BotID) {
			continue
		}
		if isBotMentionedInMessage(cfg, msg) {
			result = append(result, cfg)
		}
	}
	return result
}

func isBotMentionedInMessage(cfg ChannelConfig, msg Message) bool {
	text := strings.TrimSpace(msg.PlainText())
	for _, candidate := range botMentionCandidates(cfg) {
		if mentionTextContains(text, candidate) {
			return true
		}
		for _, part := range msg.Parts {
			if part.Type != MessagePartMention {
				continue
			}
			if mentionTextContains(part.Text, candidate) {
				return true
			}
			if mentionMetadataMatches(part.Metadata, candidate) {
				return true
			}
		}
	}
	return false
}

func botMentionCandidates(cfg ChannelConfig) []string {
	seen := map[string]struct{}{}
	candidates := make([]string, 0, 4)
	add := func(value string) {
		value = strings.TrimSpace(strings.TrimPrefix(value, "open_id:"))
		value = strings.TrimSpace(strings.TrimPrefix(value, "user_id:"))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}
	add(readConfigString(cfg.SelfIdentity, "name"))
	add(readConfigString(cfg.SelfIdentity, "display_name"))
	add(readConfigString(cfg.SelfIdentity, "open_id"))
	add(readConfigString(cfg.SelfIdentity, "user_id"))
	add(cfg.ExternalIdentity)
	return candidates
}

func mentionTextContains(text, candidate string) bool {
	text = strings.TrimSpace(text)
	candidate = strings.TrimSpace(candidate)
	if text == "" || candidate == "" {
		return false
	}
	if strings.Contains(text, "@"+candidate) {
		return true
	}
	if strings.Contains(text, `<at user_id="`+candidate+`"`) ||
		strings.Contains(text, `<at open_id="`+candidate+`"`) {
		return true
	}
	return false
}

func mentionMetadataMatches(metadata map[string]any, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || len(metadata) == 0 {
		return false
	}
	for _, key := range []string{"open_id", "user_id", "target"} {
		value := strings.TrimSpace(ReadString(metadata, key))
		value = strings.TrimPrefix(value, "open_id:")
		value = strings.TrimPrefix(value, "user_id:")
		if strings.TrimSpace(value) == candidate {
			return true
		}
	}
	return false
}

func botSenderIdentity(cfg ChannelConfig) Identity {
	openID := strings.TrimSpace(readConfigString(cfg.SelfIdentity, "open_id"))
	userID := strings.TrimSpace(readConfigString(cfg.SelfIdentity, "user_id"))
	external := strings.TrimSpace(cfg.ExternalIdentity)
	external = strings.TrimPrefix(external, "open_id:")
	external = strings.TrimPrefix(external, "user_id:")
	subjectID := openID
	if subjectID == "" {
		subjectID = external
	}
	if subjectID == "" {
		subjectID = userID
	}
	if subjectID == "" {
		subjectID = strings.TrimSpace(cfg.BotID)
	}
	attrs := map[string]string{
		"bot_id":            strings.TrimSpace(cfg.BotID),
		"channel_config_id": strings.TrimSpace(cfg.ID),
	}
	if openID != "" {
		attrs["open_id"] = openID
	}
	if userID != "" {
		attrs["user_id"] = userID
	}
	displayName := readConfigString(cfg.SelfIdentity, "name")
	if displayName == "" {
		displayName = readConfigString(cfg.SelfIdentity, "display_name")
	}
	return Identity{
		SubjectID:   subjectID,
		DisplayName: displayName,
		Attributes:  attrs,
	}
}

func botMentionTargetRef(cfg ChannelConfig) string {
	if openID := readConfigString(cfg.SelfIdentity, "open_id"); openID != "" {
		return "open_id:" + openID
	}
	if userID := readConfigString(cfg.SelfIdentity, "user_id"); userID != "" {
		return "user_id:" + userID
	}
	external := strings.TrimSpace(cfg.ExternalIdentity)
	if external == "" {
		return ""
	}
	if strings.HasPrefix(external, "open_id:") || strings.HasPrefix(external, "user_id:") {
		return external
	}
	return "open_id:" + external
}

func readConfigString(config map[string]any, key string) string {
	if len(config) == 0 {
		return ""
	}
	return strings.TrimSpace(ReadString(config, key))
}
