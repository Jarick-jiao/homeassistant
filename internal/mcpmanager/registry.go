package mcpmanager

import (
	"fmt"
	"sync"
)

// Category represents the functional category of an MCP server.
type Category string

const (
	CategoryWeather   Category = "weather"
	CategoryCalendar  Category = "calendar"
	CategoryHealth    Category = "health"
	CategoryFlight    Category = "flight"
	CategoryMap       Category = "map"
	CategoryIoT       Category = "iot"
	CategoryFood      Category = "food"
	CategoryKnowledge Category = "knowledge"
)

// ServerConfig holds configuration and metadata for an MCP server.
type ServerConfig struct {
	Name        string            `json:"name"`
	Transport   string            `json:"transport"` // "stdio" or "sse"
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         []string          `json:"env,omitempty"`
	SSEURL      string            `json:"sse_url,omitempty"`
	Categories  []Category        `json:"categories"`
	Tools       []string          `json:"tools,omitempty"` // known tool names
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Registry maintains a list of configured MCP servers with their capabilities.
type Registry struct {
	servers map[string]*ServerConfig
	mu      sync.RWMutex
}

// NewRegistry creates a new MCP server registry.
func NewRegistry() *Registry {
	return &Registry{
		servers: make(map[string]*ServerConfig),
	}
}

// Register adds a new MCP server configuration to the registry.
func (r *Registry) Register(cfg ServerConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cfg.Name == "" {
		return fmt.Errorf("server name is required")
	}

	r.servers[cfg.Name] = &cfg
	return nil
}

// Unregister removes an MCP server from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.servers, name)
}

// Get retrieves a server configuration by name.
func (r *Registry) Get(name string) (*ServerConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.servers[name]
	return s, ok
}

// GetToolsForCategory returns all server configurations that belong to a given category.
func (r *Registry) GetToolsForCategory(category Category) []*ServerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ServerConfig
	for _, s := range r.servers {
		for _, c := range s.Categories {
			if c == category {
				result = append(result, s)
				break
			}
		}
	}
	return result
}

// GetAllTools returns all registered server configurations.
func (r *Registry) GetAllTools() []*ServerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ServerConfig, 0, len(r.servers))
	for _, s := range r.servers {
		result = append(result, s)
	}
	return result
}

// ListCategories returns all unique categories present in the registry.
func (r *Registry) ListCategories() []Category {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[Category]struct{})
	for _, s := range r.servers {
		for _, c := range s.Categories {
			seen[c] = struct{}{}
		}
	}

	result := make([]Category, 0, len(seen))
	for c := range seen {
		result = append(result, c)
	}
	return result
}
