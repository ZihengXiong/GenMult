package whatsapp

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

const Type channel.ChannelType = "whatsapp"

// Config carries the resolved WhatsApp adapter configuration.
type Config struct {
	StorePath  string
	SessionJID string
	PushName   string
	// Proxy is an optional outbound proxy URL applied to both the WhatsApp
	// websocket and media transports. Supports http://, https:// and socks5://.
	Proxy string
	// ClientName is the display name announced to WhatsApp ("Browser (OS)").
	ClientName string
}

// UserConfig is the per-user binding payload.
type UserConfig struct {
	JID string
}

func normalizeConfig(raw map[string]any) (map[string]any, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"storePath": cfg.StorePath,
	}
	if cfg.SessionJID != "" {
		out["sessionJid"] = cfg.SessionJID
	}
	if cfg.PushName != "" {
		out["pushName"] = cfg.PushName
	}
	if cfg.Proxy != "" {
		out["proxy"] = cfg.Proxy
	}
	if cfg.ClientName != "" {
		out["clientName"] = cfg.ClientName
	}
	return out, nil
}

func parseConfig(raw map[string]any) (Config, error) {
	storePath := strings.TrimSpace(channel.ReadString(raw, "storePath", "store_path"))
	if storePath == "" {
		return Config{}, errors.New("whatsapp storePath is required; use QR login to create a session")
	}
	cfg := Config{
		StorePath:  filepath.Clean(storePath),
		SessionJID: strings.TrimSpace(channel.ReadString(raw, "sessionJid", "session_jid")),
		PushName:   strings.TrimSpace(channel.ReadString(raw, "pushName", "push_name")),
		Proxy:      strings.TrimSpace(channel.ReadString(raw, "proxy", "proxy_url", "proxyUrl")),
		ClientName: strings.TrimSpace(channel.ReadString(raw, "clientName", "client_name")),
	}
	if cfg.Proxy != "" {
		if err := validateProxyURL(cfg.Proxy); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func validateProxyURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("whatsapp proxy must be a valid URL (http://, https:// or socks5://)")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5":
		return nil
	default:
		return errors.New("whatsapp proxy scheme must be http, https or socks5")
	}
}

func normalizeUserConfig(raw map[string]any) (map[string]any, error) {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return nil, err
	}
	return map[string]any{"jid": cfg.JID}, nil
}

func parseUserConfig(raw map[string]any) (UserConfig, error) {
	jid := normalizeTarget(channel.ReadString(raw, "jid", "phone", "chat_id", "chatId"))
	if jid == "" {
		return UserConfig{}, errors.New("whatsapp user config requires jid, phone, or chat_id")
	}
	return UserConfig{JID: jid}, nil
}

func resolveTarget(raw map[string]any) (string, error) {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return "", err
	}
	return cfg.JID, nil
}

func normalizeTarget(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "whatsapp:")
	value = strings.TrimPrefix(value, "wa:")
	value = strings.TrimPrefix(value, "https://wa.me/")
	value = strings.TrimPrefix(value, "http://wa.me/")
	value = strings.TrimPrefix(value, "wa.me/")
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "@") {
		return value
	}
	number := normalizePhone(value)
	if number == "" {
		return value
	}
	return number + "@s.whatsapp.net"
}

func normalizePhone(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func matchBinding(raw map[string]any, criteria channel.BindingCriteria) bool {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return false
	}
	candidates := []string{
		criteria.SubjectID,
		criteria.Attribute("jid"),
		criteria.Attribute("sender_jid"),
		criteria.Attribute("chat_jid"),
		criteria.Attribute("phone"),
	}
	for _, candidate := range candidates {
		if strings.EqualFold(normalizeTarget(candidate), cfg.JID) {
			return true
		}
	}
	return false
}

func buildUserConfig(identity channel.Identity) map[string]any {
	out := map[string]any{}
	if jid := normalizeTarget(identity.SubjectID); jid != "" {
		out["jid"] = jid
		return out
	}
	for _, key := range []string{"jid", "sender_jid", "chat_jid", "phone"} {
		if jid := normalizeTarget(identity.Attribute(key)); jid != "" {
			out["jid"] = jid
			return out
		}
	}
	return out
}
