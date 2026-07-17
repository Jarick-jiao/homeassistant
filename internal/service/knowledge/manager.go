package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/homemate/server/internal/mcpmanager"
)

// Document represents a knowledge base document.
type Document struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Category  string    `json:"category"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StudyRecord represents a child's study record from dictionary pen data.
type StudyRecord struct {
	ID           string    `json:"id"`
	MemberID     string    `json:"member_id"`
	ChildName    string    `json:"child_name"`
	Subject      string    `json:"subject"`
	Activity     string    `json:"activity"` // e.g., "word_lookup", "reading", "listening"
	Details      string    `json:"details"`
	DurationMin  int       `json:"duration_min"`
	Score        int       `json:"score,omitempty"`
	RecordedAt   time.Time `json:"recorded_at"`
}

// KnowledgeManager provides knowledge base management functionality.
type KnowledgeManager struct {
	mcpManager *mcpmanager.MCPClientManager
	registry   *mcpmanager.Registry
	store      KnowledgeStore
}

// KnowledgeStore defines the interface for knowledge data persistence.
type KnowledgeStore interface {
	SaveDocument(ctx context.Context, doc *Document) error
	SearchDocuments(ctx context.Context, query string, category string) ([]Document, error)
	GetDocumentByID(ctx context.Context, id string) (*Document, error)
	DeleteDocument(ctx context.Context, id string) error
	SaveStudyRecord(ctx context.Context, record *StudyRecord) error
	GetStudyRecordsByMember(ctx context.Context, memberID string, start, end time.Time) ([]StudyRecord, error)
}

// NewKnowledgeManager creates a new knowledge manager.
func NewKnowledgeManager(mcpManager *mcpmanager.MCPClientManager, registry *mcpmanager.Registry, store KnowledgeStore) *KnowledgeManager {
	return &KnowledgeManager{
		mcpManager: mcpManager,
		registry:   registry,
		store:      store,
	}
}

// AddDocument adds a new document to the knowledge base.
func (k *KnowledgeManager) AddDocument(ctx context.Context, title, content, category string) (*Document, error) {
	if title == "" || content == "" {
		return nil, fmt.Errorf("title and content are required")
	}

	doc := &Document{
		ID:        generateDocID(),
		Title:     title,
		Content:   content,
		Category:  category,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := k.store.SaveDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("save document failed: %w", err)
	}

	// Optionally index via MCP knowledge server
	servers := k.registry.GetToolsForCategory(mcpmanager.CategoryKnowledge)
	if len(servers) > 0 {
		_, _ = k.mcpManager.CallTool(ctx, servers[0].Name, "indexDocument", map[string]interface{}{
			"id":       doc.ID,
			"title":    doc.Title,
			"content":  doc.Content,
			"category": doc.Category,
		})
	}

	return doc, nil
}

// SearchKnowledge searches the knowledge base for documents matching the query.
func (k *KnowledgeManager) SearchKnowledge(ctx context.Context, query string) ([]Document, error) {
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	// First try local search
	docs, err := k.store.SearchDocuments(ctx, query, "")
	if err != nil {
		return nil, fmt.Errorf("search documents failed: %w", err)
	}

	// Also try MCP knowledge server if available
	servers := k.registry.GetToolsForCategory(mcpmanager.CategoryKnowledge)
	if len(servers) > 0 {
		result, err := k.mcpManager.CallTool(ctx, servers[0].Name, "searchDocuments", map[string]interface{}{
			"query": query,
		})
		if err == nil {
			_ = result
			// TODO: merge MCP results with local results
		}
	}

	return docs, nil
}

// GetChildStudyRecords retrieves study records for a child member.
func (k *KnowledgeManager) GetChildStudyRecords(ctx context.Context, memberID string) ([]StudyRecord, error) {
	end := time.Now()
	start := end.AddDate(0, 0, -30)

	records, err := k.store.GetStudyRecordsByMember(ctx, memberID, start, end)
	if err != nil {
		return nil, fmt.Errorf("fetch study records failed: %w", err)
	}

	return records, nil
}

// ImportDictionaryPenData imports study data from a dictionary pen device.
func (k *KnowledgeManager) ImportDictionaryPenData(ctx context.Context, memberID string, rawData []byte) error {
	// Parse raw data (format depends on device vendor)
	// For demo, assume JSON array of records
	lines := strings.Split(string(rawData), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		record := &StudyRecord{
			ID:          generateDocID(),
			MemberID:    memberID,
			Activity:    "word_lookup",
			Details:     line,
			RecordedAt:  time.Now(),
			DurationMin: 5,
		}

		if err := k.store.SaveStudyRecord(ctx, record); err != nil {
			return fmt.Errorf("save study record failed: %w", err)
		}
	}

	return nil
}

// generateDocID generates a simple unique ID for documents.
func generateDocID() string {
	return fmt.Sprintf("doc_%d", time.Now().UnixNano())
}
