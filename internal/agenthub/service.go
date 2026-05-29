package agenthub

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

var (
	ErrNotFound      = errors.New("agent hub room not found")
	ErrInvalidRoomID = errors.New("invalid room id")
	ErrInvalidOwner  = errors.New("invalid owner user id")
)

type Service struct {
	queries dbstore.Queries
}

type Room struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	ShortName   string         `json:"short_name"`
	Subtitle    string         `json:"subtitle"`
	Summary     string         `json:"summary"`
	Members     int            `json:"members"`
	Attention   int            `json:"attention"`
	Privacy     string         `json:"privacy"`
	Live        string         `json:"live"`
	Accent      string         `json:"accent"`
	StatusClass string         `json:"status_class"`
	AgentIDs    []string       `json:"agent_ids"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type ListRoomsResponse struct {
	Items []Room `json:"items"`
}

type Message struct {
	ID         string         `json:"id"`
	RoomID     string         `json:"room_id"`
	SenderID   string         `json:"sender_id"`
	SenderType string         `json:"sender_type"`
	SenderName string         `json:"sender_name"`
	Kind       string         `json:"kind"`
	Title      string         `json:"title"`
	Body       string         `json:"body"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type ListMessagesResponse struct {
	Items []Message `json:"items"`
}

type UpsertRoomRequest struct {
	Name        string         `json:"name"`
	ShortName   string         `json:"short_name"`
	Subtitle    string         `json:"subtitle"`
	Summary     string         `json:"summary"`
	Members     int            `json:"members"`
	Attention   int            `json:"attention"`
	Privacy     string         `json:"privacy"`
	Live        string         `json:"live"`
	Accent      string         `json:"accent"`
	StatusClass string         `json:"status_class"`
	AgentIDs    []string       `json:"agent_ids"`
	Metadata    map[string]any `json:"metadata"`
}

type AddAgentRequest struct {
	AgentID string `json:"agent_id"`
}

type CreateMessageRequest struct {
	SenderID   string         `json:"sender_id"`
	SenderType string         `json:"sender_type"`
	SenderName string         `json:"sender_name"`
	Kind       string         `json:"kind"`
	Title      string         `json:"title"`
	Body       string         `json:"body"`
	Metadata   map[string]any `json:"metadata"`
}

func NewService(queries dbstore.Queries) *Service {
	return &Service{queries: queries}
}

func (s *Service) List(ctx context.Context, ownerUserID string) (ListRoomsResponse, error) {
	ownerID, err := parseOwnerID(ownerUserID)
	if err != nil {
		return ListRoomsResponse{}, err
	}
	rows, err := s.queries.ListAgentHubRooms(ctx, ownerID)
	if err != nil {
		return ListRoomsResponse{}, err
	}
	agentMap, err := s.agentIDsByRoom(ctx, ownerID)
	if err != nil {
		return ListRoomsResponse{}, err
	}
	items := make([]Room, 0, len(rows))
	for _, row := range rows {
		items = append(items, roomFromRow(row, agentMap[uuidString(row.ID)]))
	}
	return ListRoomsResponse{Items: items}, nil
}

func (s *Service) Get(ctx context.Context, ownerUserID, roomID string) (Room, error) {
	ownerID, roomUUID, err := parseOwnerAndRoom(ownerUserID, roomID)
	if err != nil {
		return Room{}, err
	}
	row, err := s.queries.GetAgentHubRoom(ctx, dbsqlc.GetAgentHubRoomParams{
		ID:          roomUUID,
		OwnerUserID: ownerID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Room{}, ErrNotFound
		}
		return Room{}, err
	}
	agentMap, err := s.agentIDsByRoom(ctx, ownerID)
	if err != nil {
		return Room{}, err
	}
	return roomFromRow(row, agentMap[uuidString(row.ID)]), nil
}

func (s *Service) Create(ctx context.Context, ownerUserID string, req UpsertRoomRequest) (Room, error) {
	ownerID, err := parseOwnerID(ownerUserID)
	if err != nil {
		return Room{}, err
	}
	req = normalizeRoomRequest(req)
	if req.Name == "" {
		return Room{}, errors.New("name is required")
	}
	row, err := s.queries.CreateAgentHubRoom(ctx, dbsqlc.CreateAgentHubRoomParams{
		OwnerUserID: ownerID,
		Name:        req.Name,
		ShortName:   req.ShortName,
		Subtitle:    req.Subtitle,
		Summary:     req.Summary,
		Privacy:     req.Privacy,
		Live:        req.Live,
		Accent:      req.Accent,
		StatusClass: req.StatusClass,
		Attention:   int32(req.Attention),
		Metadata:    metadataBytes(req),
	})
	if err != nil {
		return Room{}, err
	}
	roomID := row.ID
	for _, agentID := range normalizedAgentIDs(req.AgentIDs) {
		if err := s.queries.AddAgentHubRoomAgent(ctx, dbsqlc.AddAgentHubRoomAgentParams{RoomID: roomID, AgentID: agentID}); err != nil {
			return Room{}, err
		}
	}
	if _, err := s.createMessage(ctx, roomID, CreateMessageRequest{
		SenderType: "system",
		SenderName: "AgentHub",
		Kind:       "room",
		Title:      req.Name + " is ready",
		Body:       "This room is persisted on the backend. Member changes and collaboration messages will be written to this timeline.",
	}); err != nil {
		return Room{}, err
	}
	return s.Get(ctx, ownerUserID, uuidString(roomID))
}

func (s *Service) Update(ctx context.Context, ownerUserID, roomID string, req UpsertRoomRequest) (Room, error) {
	ownerID, roomUUID, err := parseOwnerAndRoom(ownerUserID, roomID)
	if err != nil {
		return Room{}, err
	}
	req = normalizeRoomRequest(req)
	if req.Name == "" {
		return Room{}, errors.New("name is required")
	}
	_, err = s.queries.UpdateAgentHubRoom(ctx, dbsqlc.UpdateAgentHubRoomParams{
		ID:          roomUUID,
		OwnerUserID: ownerID,
		Name:        req.Name,
		ShortName:   req.ShortName,
		Subtitle:    req.Subtitle,
		Summary:     req.Summary,
		Privacy:     req.Privacy,
		Live:        req.Live,
		Accent:      req.Accent,
		StatusClass: req.StatusClass,
		Attention:   int32(req.Attention),
		Metadata:    metadataBytes(req),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Room{}, ErrNotFound
		}
		return Room{}, err
	}
	current, err := s.Get(ctx, ownerUserID, roomID)
	if err != nil {
		return Room{}, err
	}
	want := make(map[string]struct{})
	for _, agentID := range normalizedAgentIDs(req.AgentIDs) {
		want[agentID] = struct{}{}
	}
	for _, agentID := range current.AgentIDs {
		if _, ok := want[agentID]; ok {
			delete(want, agentID)
			continue
		}
		if err := s.queries.DeleteAgentHubRoomAgent(ctx, dbsqlc.DeleteAgentHubRoomAgentParams{RoomID: roomUUID, AgentID: agentID}); err != nil {
			return Room{}, err
		}
	}
	for agentID := range want {
		if err := s.queries.AddAgentHubRoomAgent(ctx, dbsqlc.AddAgentHubRoomAgentParams{RoomID: roomUUID, AgentID: agentID}); err != nil {
			return Room{}, err
		}
	}
	return s.Get(ctx, ownerUserID, roomID)
}

func (s *Service) Delete(ctx context.Context, ownerUserID, roomID string) error {
	ownerID, roomUUID, err := parseOwnerAndRoom(ownerUserID, roomID)
	if err != nil {
		return err
	}
	return s.queries.DeleteAgentHubRoom(ctx, dbsqlc.DeleteAgentHubRoomParams{ID: roomUUID, OwnerUserID: ownerID})
}

func (s *Service) AddAgent(ctx context.Context, ownerUserID, roomID, agentID string) (Room, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return Room{}, errors.New("agent_id is required")
	}
	if _, err := s.Get(ctx, ownerUserID, roomID); err != nil {
		return Room{}, err
	}
	_, roomUUID, err := parseOwnerAndRoom(ownerUserID, roomID)
	if err != nil {
		return Room{}, err
	}
	if err := s.queries.AddAgentHubRoomAgent(ctx, dbsqlc.AddAgentHubRoomAgentParams{RoomID: roomUUID, AgentID: agentID}); err != nil {
		return Room{}, err
	}
	_, _ = s.createMessage(ctx, roomUUID, CreateMessageRequest{
		SenderID:   agentID,
		SenderType: "system",
		SenderName: "AgentHub",
		Kind:       "member",
		Title:      "Member added",
		Body:       agentID + " joined this room.",
	})
	return s.Get(ctx, ownerUserID, roomID)
}

func (s *Service) RemoveAgent(ctx context.Context, ownerUserID, roomID, agentID string) (Room, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return Room{}, errors.New("agent_id is required")
	}
	if _, err := s.Get(ctx, ownerUserID, roomID); err != nil {
		return Room{}, err
	}
	_, roomUUID, err := parseOwnerAndRoom(ownerUserID, roomID)
	if err != nil {
		return Room{}, err
	}
	if err := s.queries.DeleteAgentHubRoomAgent(ctx, dbsqlc.DeleteAgentHubRoomAgentParams{RoomID: roomUUID, AgentID: agentID}); err != nil {
		return Room{}, err
	}
	_, _ = s.createMessage(ctx, roomUUID, CreateMessageRequest{
		SenderID:   agentID,
		SenderType: "system",
		SenderName: "AgentHub",
		Kind:       "member",
		Title:      "Member removed",
		Body:       agentID + " left this room.",
	})
	return s.Get(ctx, ownerUserID, roomID)
}

func (s *Service) ListMessages(ctx context.Context, ownerUserID, roomID string, limit int32) (ListMessagesResponse, error) {
	ownerID, roomUUID, err := parseOwnerAndRoom(ownerUserID, roomID)
	if err != nil {
		return ListMessagesResponse{}, err
	}
	if _, err := s.Get(ctx, ownerUserID, roomID); err != nil {
		return ListMessagesResponse{}, err
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.queries.ListAgentHubRoomMessages(ctx, dbsqlc.ListAgentHubRoomMessagesParams{
		RoomID:      roomUUID,
		OwnerUserID: ownerID,
		LimitCount:  limit,
	})
	if err != nil {
		return ListMessagesResponse{}, err
	}
	items := make([]Message, 0, len(rows))
	for _, row := range rows {
		items = append(items, messageFromRow(row))
	}
	return ListMessagesResponse{Items: items}, nil
}

func (s *Service) CreateMessage(ctx context.Context, ownerUserID, roomID string, req CreateMessageRequest) (Message, error) {
	_, roomUUID, err := parseOwnerAndRoom(ownerUserID, roomID)
	if err != nil {
		return Message{}, err
	}
	if _, err := s.Get(ctx, ownerUserID, roomID); err != nil {
		return Message{}, err
	}
	return s.createMessage(ctx, roomUUID, req)
}

func (s *Service) createMessage(ctx context.Context, roomUUID pgtype.UUID, req CreateMessageRequest) (Message, error) {
	req = normalizeMessageRequest(req)
	if req.Body == "" {
		return Message{}, errors.New("body is required")
	}
	row, err := s.queries.CreateAgentHubRoomMessage(ctx, dbsqlc.CreateAgentHubRoomMessageParams{
		RoomID:     roomUUID,
		SenderID:   req.SenderID,
		SenderType: req.SenderType,
		SenderName: req.SenderName,
		Kind:       req.Kind,
		Title:      req.Title,
		Body:       req.Body,
		Metadata:   jsonBytes(req.Metadata),
	})
	if err != nil {
		return Message{}, err
	}
	return messageFromRow(row), nil
}

func (s *Service) agentIDsByRoom(ctx context.Context, ownerID pgtype.UUID) (map[string][]string, error) {
	rows, err := s.queries.ListAgentHubRoomAgentsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string)
	for _, row := range rows {
		key := uuidString(row.RoomID)
		out[key] = append(out[key], row.AgentID)
	}
	return out, nil
}

func normalizeMessageRequest(req CreateMessageRequest) CreateMessageRequest {
	req.SenderID = strings.TrimSpace(req.SenderID)
	req.SenderType = strings.TrimSpace(req.SenderType)
	req.SenderName = strings.TrimSpace(req.SenderName)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	if req.SenderType == "" {
		req.SenderType = "user"
	}
	if req.SenderName == "" {
		req.SenderName = "Me"
	}
	if req.Kind == "" {
		req.Kind = "message"
	}
	if req.Title == "" {
		req.Title = req.SenderName
	}
	return req
}

func normalizeRoomRequest(req UpsertRoomRequest) UpsertRoomRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.ShortName = strings.TrimSpace(req.ShortName)
	req.Subtitle = strings.TrimSpace(req.Subtitle)
	req.Summary = strings.TrimSpace(req.Summary)
	req.Privacy = strings.TrimSpace(req.Privacy)
	req.Live = strings.TrimSpace(req.Live)
	req.Accent = strings.TrimSpace(req.Accent)
	req.StatusClass = strings.TrimSpace(req.StatusClass)
	if req.ShortName == "" {
		req.ShortName = roomShortName(req.Name)
	}
	if req.Subtitle == "" {
		req.Subtitle = "Custom Agent room"
	}
	if req.Summary == "" {
		req.Summary = "This room has no description yet. Add agents first, then start collaboration tasks from here."
	}
	if req.Privacy == "" {
		req.Privacy = "Local room"
	}
	if req.Live == "" {
		req.Live = "Waiting for input"
	}
	if req.Accent == "" {
		req.Accent = "bg-slate-700"
	}
	if req.StatusClass == "" {
		req.StatusClass = "bg-slate-500"
	}
	if req.Members <= 0 {
		req.Members = 1
	}
	req.AgentIDs = normalizedAgentIDs(req.AgentIDs)
	return req
}

func metadataBytes(req UpsertRoomRequest) []byte {
	metadata := map[string]any{}
	for k, v := range req.Metadata {
		metadata[k] = v
	}
	metadata["members"] = req.Members
	data, err := json.Marshal(metadata)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func jsonBytes(value map[string]any) []byte {
	if value == nil {
		return []byte("{}")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func roomFromRow(row dbsqlc.AgentHubRoom, agentIDs []string) Room {
	metadata := map[string]any{}
	if len(row.Metadata) > 0 {
		_ = json.Unmarshal(row.Metadata, &metadata)
	}
	members := 1
	if raw, ok := metadata["members"]; ok {
		switch value := raw.(type) {
		case float64:
			if value > 0 {
				members = int(value)
			}
		case int:
			if value > 0 {
				members = value
			}
		}
	}
	return Room{
		ID:          uuidString(row.ID),
		Name:        row.Name,
		ShortName:   row.ShortName,
		Subtitle:    row.Subtitle,
		Summary:     row.Summary,
		Members:     members,
		Attention:   int(row.Attention),
		Privacy:     row.Privacy,
		Live:        row.Live,
		Accent:      row.Accent,
		StatusClass: row.StatusClass,
		AgentIDs:    normalizedAgentIDs(agentIDs),
		Metadata:    metadata,
		CreatedAt:   dbpkg.TimeFromPg(row.CreatedAt),
		UpdatedAt:   dbpkg.TimeFromPg(row.UpdatedAt),
	}
}

func messageFromRow(row dbsqlc.AgentHubRoomMessage) Message {
	metadata := map[string]any{}
	if len(row.Metadata) > 0 {
		_ = json.Unmarshal(row.Metadata, &metadata)
	}
	return Message{
		ID:         uuidString(row.ID),
		RoomID:     uuidString(row.RoomID),
		SenderID:   row.SenderID,
		SenderType: row.SenderType,
		SenderName: row.SenderName,
		Kind:       row.Kind,
		Title:      row.Title,
		Body:       row.Body,
		Metadata:   metadata,
		CreatedAt:  dbpkg.TimeFromPg(row.CreatedAt),
	}
}

func parseOwnerAndRoom(ownerUserID, roomID string) (pgtype.UUID, pgtype.UUID, error) {
	ownerID, err := parseOwnerID(ownerUserID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	roomUUID, err := dbpkg.ParseUUID(roomID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, ErrInvalidRoomID
	}
	return ownerID, roomUUID, nil
}

func parseOwnerID(ownerUserID string) (pgtype.UUID, error) {
	ownerID, err := dbpkg.ParseUUID(ownerUserID)
	if err != nil {
		return pgtype.UUID{}, ErrInvalidOwner
	}
	return ownerID, nil
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	parsed, err := uuid.FromBytes(id.Bytes[:])
	if err != nil {
		return ""
	}
	return parsed.String()
}

func normalizedAgentIDs(agentIDs []string) []string {
	seen := make(map[string]struct{}, len(agentIDs))
	out := make([]string, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		seen[agentID] = struct{}{}
		out = append(out, agentID)
	}
	return out
}

func roomShortName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "群"
	}
	runes := []rune(name)
	return strings.ToUpper(string(runes[:1]))
}
