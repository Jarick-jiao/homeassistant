package calendar

import (
	"context"
	"fmt"
	"time"

	"github.com/homemate/server/internal/mcpmanager"
)

// EventSource represents the source of calendar events.
type EventSource string

const (
	SourceGoogle   EventSource = "google"
	SourceLocal    EventSource = "local"
	SourceOutlook  EventSource = "outlook"
)

// CalendarEvent represents a calendar event.
type CalendarEvent struct {
	ID          string      `json:"id"`
	MemberID    string      `json:"member_id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	StartTime   time.Time   `json:"start_time"`
	EndTime     time.Time   `json:"end_time"`
	Location    string      `json:"location"`
	Source      EventSource `json:"source"`
	IsAllDay    bool        `json:"is_all_day"`
	ReminderMin int         `json:"reminder_min"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// CalendarSync provides calendar synchronization functionality.
type CalendarSync struct {
	mcpManager *mcpmanager.MCPClientManager
	registry   *mcpmanager.Registry
	store      CalendarStore
}

// CalendarStore defines the interface for persisting calendar events.
type CalendarStore interface {
	SaveEvent(ctx context.Context, event *CalendarEvent) error
	GetEventsByMember(ctx context.Context, memberID string, start, end time.Time) ([]CalendarEvent, error)
	DeleteEvent(ctx context.Context, eventID string) error
	GetEventByID(ctx context.Context, eventID string) (*CalendarEvent, error)
}

// NewCalendarSync creates a new calendar sync service.
func NewCalendarSync(mcpManager *mcpmanager.MCPClientManager, registry *mcpmanager.Registry, store CalendarStore) *CalendarSync {
	return &CalendarSync{
		mcpManager: mcpManager,
		registry:   registry,
		store:      store,
	}
}

// SyncEvents syncs calendar events from the specified source.
func (c *CalendarSync) SyncEvents(ctx context.Context, memberID string, source EventSource) error {
	servers := c.registry.GetToolsForCategory(mcpmanager.CategoryCalendar)
	if len(servers) == 0 {
		return fmt.Errorf("no calendar server available")
	}

	var serverName string
	for _, s := range servers {
		if s.Metadata["source"] == string(source) {
			serverName = s.Name
			break
		}
	}
	if serverName == "" {
		serverName = servers[0].Name
	}

	now := time.Now()
	start := now.AddDate(0, 0, -30)
	end := now.AddDate(0, 0, 90)

	result, err := c.mcpManager.CallTool(ctx, serverName, "listEvents", map[string]interface{}{
		"member_id":  memberID,
		"start_date": start.Format("2006-01-02"),
		"end_date":   end.Format("2006-01-02"),
	})
	if err != nil {
		return fmt.Errorf("sync events from %s failed: %w", source, err)
	}

	_ = result
	// TODO: parse result content and persist events via c.store
	return nil
}

// CreateEvent creates a new calendar event locally and optionally syncs to external source.
func (c *CalendarSync) CreateEvent(ctx context.Context, event *CalendarEvent) error {
	if event.Title == "" {
		return fmt.Errorf("event title is required")
	}

	now := time.Now()
	event.CreatedAt = now
	event.UpdatedAt = now

	if err := c.store.SaveEvent(ctx, event); err != nil {
		return fmt.Errorf("save event failed: %w", err)
	}

	// Optionally sync to external calendar via MCP
	if event.Source == SourceGoogle {
		servers := c.registry.GetToolsForCategory(mcpmanager.CategoryCalendar)
		if len(servers) > 0 {
			_, _ = c.mcpManager.CallTool(ctx, servers[0].Name, "createEvent", map[string]interface{}{
				"title":       event.Title,
				"description": event.Description,
				"start_time":  event.StartTime.Format(time.RFC3339),
				"end_time":    event.EndTime.Format(time.RFC3339),
				"location":    event.Location,
			})
		}
	}

	return nil
}

// GetUpcomingEvents retrieves upcoming events for a member within the specified days.
func (c *CalendarSync) GetUpcomingEvents(ctx context.Context, memberID string, days int) ([]CalendarEvent, error) {
	now := time.Now()
	end := now.AddDate(0, 0, days)

	events, err := c.store.GetEventsByMember(ctx, memberID, now, end)
	if err != nil {
		return nil, fmt.Errorf("fetch upcoming events failed: %w", err)
	}

	return events, nil
}
