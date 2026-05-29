package whatsapp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/config"
)

// QRHandler manages WhatsApp linking sessions (QR + pair-code) for the
// management UI.
type QRHandler struct {
	logger    *slog.Logger
	lifecycle *channel.Lifecycle
	adapter   *WhatsAppAdapter

	mu       sync.Mutex
	sessions map[string]*qrLoginSession
}

type qrLoginSession struct {
	botID      string
	storePath  string
	proxy      string
	clientName string
	container  *sqlstore.Container
	client     *whatsmeow.Client
	events     <-chan whatsmeow.QRChannelItem
	cancel     context.CancelFunc
	pairCode   string
}

// NewQRHandler builds a QR handler and the underlying WhatsApp adapter.
func NewQRHandler(log *slog.Logger, lifecycle *channel.Lifecycle, cfg config.Config) *QRHandler {
	if log == nil {
		log = slog.Default()
	}
	return &QRHandler{
		logger:    log.With(slog.String("handler", "whatsapp_qr")),
		lifecycle: lifecycle,
		adapter:   NewWhatsAppAdapter(log, cfg.Workspace.DataRoot),
		sessions:  map[string]*qrLoginSession{},
	}
}

// NewQRServerHandler is the DI-friendly constructor used by fx.
func NewQRServerHandler(log *slog.Logger, lifecycle *channel.Lifecycle, cfg config.Config) *QRHandler {
	return NewQRHandler(log, lifecycle, cfg)
}

// Register attaches the WhatsApp linking endpoints onto the Echo router.
func (h *QRHandler) Register(e *echo.Echo) {
	e.POST("/bots/:id/channel/whatsapp/qr/start", h.Start)
	e.POST("/bots/:id/channel/whatsapp/qr/poll", h.Poll)
	e.POST("/bots/:id/channel/whatsapp/pair", h.Pair)
}

// QRStartResponse is the body returned by Start.
type QRStartResponse struct {
	QRCode  string `json:"qr_code,omitempty"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

// QRStartRequest carries optional transport/linking settings from the
// credentials form. This lets proxy-based logins work before credentials are
// persisted.
type QRStartRequest struct {
	Proxy      string `json:"proxy,omitempty"`
	ClientName string `json:"clientName,omitempty"`
}

// Start godoc
// @Summary Start WhatsApp QR login
// @Description Open a fresh whatsmeow session and return the first QR code event.
// @Tags bots
// @Param id path string true "Bot ID"
// @Param payload body QRStartRequest false "Optional proxy/client display name"
// @Success 200 {object} QRStartResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /bots/{id}/channel/whatsapp/qr/start [post].
func (h *QRHandler) Start(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	var req QRStartRequest
	if c.Request().ContentLength != 0 {
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	h.closeSession(botID)

	storePath := h.adapter.defaultStorePath(botID)
	proxy := strings.TrimSpace(req.Proxy)
	if proxy != "" {
		if err := validateProxyURL(proxy); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	clientName := normalizeClientName(req.ClientName)
	loginCtx, cancel := context.WithCancel(context.WithoutCancel(c.Request().Context()))
	container, client, err := h.adapter.openClient(loginCtx, Config{StorePath: storePath, Proxy: proxy, ClientName: clientName})
	if err != nil {
		cancel()
		h.logger.Error("whatsapp qr open client failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create WhatsApp client: "+err.Error())
	}
	if client.Store.ID != nil {
		client.Disconnect()
		_ = container.Close()
		cancel()
		if err := h.saveConfig(c.Request().Context(), botID, storePath, client.Store.ID.String(), "", proxy, clientName); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, QRStartResponse{Status: "confirmed", Message: "Already logged in"})
	}
	qrChan, err := client.GetQRChannel(loginCtx)
	if err != nil {
		_ = container.Close()
		cancel()
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create QR channel: "+err.Error())
	}
	if err := client.Connect(); err != nil {
		_ = container.Close()
		cancel()
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to connect WhatsApp: "+err.Error())
	}

	session := &qrLoginSession{
		botID:      botID,
		storePath:  storePath,
		proxy:      proxy,
		clientName: clientName,
		container:  container,
		client:     client,
		events:     qrChan,
		cancel:     cancel,
	}
	h.mu.Lock()
	h.sessions[botID] = session
	h.mu.Unlock()

	select {
	case evt, ok := <-qrChan:
		if !ok {
			h.closeSession(botID)
			return echo.NewHTTPError(http.StatusInternalServerError, "QR channel closed")
		}
		return h.handleQREvent(c, session, evt, true)
	case <-time.After(20 * time.Second):
		return c.JSON(http.StatusOK, QRStartResponse{Status: "wait", Message: "Waiting for QR code"})
	case <-c.Request().Context().Done():
		return c.Request().Context().Err()
	}
}

// QRPollResponse is the body returned by Poll.
type QRPollResponse struct {
	Status   string `json:"status"`
	QRCode   string `json:"qr_code,omitempty"`
	PairCode string `json:"pair_code,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Poll godoc
// @Summary Poll WhatsApp QR / pair status
// @Description Long-poll the linking session to receive the next QR refresh, scan progress, or login confirmation.
// @Tags bots
// @Param id path string true "Bot ID"
// @Success 200 {object} QRPollResponse
// @Failure 400 {object} map[string]string
// @Router /bots/{id}/channel/whatsapp/qr/poll [post].
func (h *QRHandler) Poll(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	h.mu.Lock()
	session := h.sessions[botID]
	h.mu.Unlock()
	if session == nil {
		return c.JSON(http.StatusOK, QRPollResponse{Status: "expired", Message: "QR login session expired"})
	}
	select {
	case evt, ok := <-session.events:
		if !ok {
			h.closeSession(botID)
			return c.JSON(http.StatusOK, QRPollResponse{Status: "expired", Message: "QR login session closed"})
		}
		return h.handleQREvent(c, session, evt, false)
	case <-time.After(25 * time.Second):
		return c.JSON(http.StatusOK, QRPollResponse{Status: "wait", Message: "Waiting for scan", PairCode: session.pairCode})
	case <-c.Request().Context().Done():
		return c.Request().Context().Err()
	}
}

// PairRequest is the body for Pair.
type PairRequest struct {
	Phone      string `json:"phone"`
	ClientName string `json:"clientName,omitempty"`
}

// Pair godoc
// @Summary Request a WhatsApp pairing code
// @Description Trigger pair-with-phone-number flow on an active linking session and return the 8-character code.
// @Tags bots
// @Param id path string true "Bot ID"
// @Param payload body PairRequest true "Phone number"
// @Success 200 {object} QRPollResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /bots/{id}/channel/whatsapp/pair [post].
func (h *QRHandler) Pair(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	var req PairRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	phone := normalizePhone(req.Phone)
	if phone == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "phone is required (digits only, with country code)")
	}
	h.mu.Lock()
	session := h.sessions[botID]
	h.mu.Unlock()
	if session == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no active linking session; call qr/start first")
	}
	clientName := normalizeClientName(req.ClientName)
	if clientName == "" {
		clientName = session.clientName
	}
	code, err := session.client.PairPhone(c.Request().Context(), phone, true, whatsmeow.PairClientChrome, clientNameOrDefault(clientName))
	if err != nil {
		h.logger.Error("whatsapp pair phone failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to request pair code: "+err.Error())
	}
	formatted := formatPairCode(code)
	h.mu.Lock()
	if s, ok := h.sessions[botID]; ok {
		s.pairCode = formatted
		if clientName != "" {
			s.clientName = clientName
		}
	}
	h.mu.Unlock()
	return c.JSON(http.StatusOK, QRPollResponse{
		Status:   "pair_code",
		PairCode: formatted,
		Message:  "Enter this code on your phone",
	})
}

var pairCodeFormatter = regexp.MustCompile(`^([A-Z0-9]{4})([A-Z0-9]{4})$`)

// formatPairCode adds a hyphen between the two 4-character halves so the UI can
// render "ABCD-EFGH" instead of "ABCDEFGH".
func formatPairCode(raw string) string {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if m := pairCodeFormatter.FindStringSubmatch(value); m != nil {
		return m[1] + "-" + m[2]
	}
	return value
}

func (h *QRHandler) handleQREvent(c echo.Context, session *qrLoginSession, evt whatsmeow.QRChannelItem, start bool) error {
	switch evt.Event {
	case "code":
		if start {
			return c.JSON(http.StatusOK, QRStartResponse{Status: "code", QRCode: evt.Code, Message: "Scan the QR code with WhatsApp"})
		}
		return c.JSON(http.StatusOK, QRPollResponse{Status: "code", QRCode: evt.Code, PairCode: session.pairCode, Message: "Scan the QR code with WhatsApp"})
	case "success":
		sessionJID := ""
		if session.client != nil && session.client.Store.ID != nil {
			sessionJID = session.client.Store.ID.String()
		}
		h.closeSession(session.botID)
		if err := h.saveConfig(c.Request().Context(), session.botID, session.storePath, sessionJID, "", session.proxy, session.clientName); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Login succeeded but failed to save credentials: "+err.Error())
		}
		if start {
			return c.JSON(http.StatusOK, QRStartResponse{Status: "confirmed", Message: "Login successful"})
		}
		return c.JSON(http.StatusOK, QRPollResponse{Status: "confirmed", Message: "Login successful"})
	case "timeout":
		h.closeSession(session.botID)
		if start {
			return c.JSON(http.StatusOK, QRStartResponse{Status: "expired", Message: "QR code expired"})
		}
		return c.JSON(http.StatusOK, QRPollResponse{Status: "expired", Message: "QR code expired"})
	case "err-client-outdated":
		h.closeSession(session.botID)
		err := errors.New("whatsapp client appears to be outdated; update memoh / whatsmeow dependency")
		if start {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	default:
		if start {
			return c.JSON(http.StatusOK, QRStartResponse{Status: evt.Event, Message: evt.Event})
		}
		return c.JSON(http.StatusOK, QRPollResponse{Status: evt.Event, Message: evt.Event})
	}
}

func (h *QRHandler) saveConfig(ctx context.Context, botID, storePath, sessionJID, pushName, proxy, clientName string) error {
	if h.lifecycle == nil {
		return nil
	}
	credentials := map[string]any{
		"storePath": storePath,
	}
	if strings.TrimSpace(sessionJID) != "" {
		credentials["sessionJid"] = strings.TrimSpace(sessionJID)
	}
	if strings.TrimSpace(pushName) != "" {
		credentials["pushName"] = strings.TrimSpace(pushName)
	}
	if strings.TrimSpace(proxy) != "" {
		credentials["proxy"] = strings.TrimSpace(proxy)
	}
	if strings.TrimSpace(clientName) != "" {
		credentials["clientName"] = strings.TrimSpace(clientName)
	}
	_, err := h.lifecycle.UpsertBotChannelConfig(ctx, botID, Type, channel.UpsertConfigRequest{
		Credentials: credentials,
		Disabled:    boolPtr(false),
	})
	return err
}

func (h *QRHandler) closeSession(botID string) {
	h.mu.Lock()
	session := h.sessions[botID]
	delete(h.sessions, botID)
	h.mu.Unlock()
	if session == nil {
		return
	}
	if session.cancel != nil {
		session.cancel()
	}
	if session.client != nil {
		session.client.Disconnect()
	}
	if session.container != nil {
		_ = session.container.Close()
	}
}

func boolPtr(v bool) *bool { return &v }

func normalizeClientName(raw string) string {
	return strings.TrimSpace(raw)
}

func clientNameOrDefault(raw string) string {
	if v := normalizeClientName(raw); v != "" {
		return v
	}
	return "Chrome (Linux)"
}
