package mcpmanager

import (
	"context"
	"fmt"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPClient wraps a single MCP server connection and its capabilities.
type MCPClient struct {
	Name      string
	Client    client.MCPClient
	Tools     []mcp.Tool
	Connected bool
	Transport string // "stdio" | "sse"
}

// MCPClientManager manages connections to multiple MCP servers.
type MCPClientManager struct {
	clients map[string]*MCPClient
	mu      sync.RWMutex
}

// NewMCPClientManager creates a new MCP client manager.
func NewMCPClientManager() *MCPClientManager {
	return &MCPClientManager{
		clients: make(map[string]*MCPClient),
	}
}

// Connect connects to a specified MCP server.
func (m *MCPClientManager) Connect(
	ctx context.Context,
	serverName string,
	transport string,
	command string,
	args []string,
	env []string,
	sseURL string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clients[serverName]; exists {
		return fmt.Errorf("server %s already connected", serverName)
	}

	var c client.MCPClient
	var err error

	switch transport {
	case "stdio":
		c, err = client.NewStdioMCPClient(command, env, args...)
	case "sse":
		c, err = client.NewSSEMCPClient(sseURL)
	default:
		return fmt.Errorf("unsupported transport: %s", transport)
	}

	if err != nil {
		return fmt.Errorf("failed to create mcp client for %s: %w", serverName, err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "homemate-server",
		Version: "3.0.0",
	}

	_, err = c.Initialize(ctx, initReq)
	if err != nil {
		c.Close()
		return fmt.Errorf("failed to initialize mcp client for %s: %w", serverName, err)
	}

	m.clients[serverName] = &MCPClient{
		Name:      serverName,
		Client:    c,
		Connected: true,
		Transport: transport,
	}

	return nil
}

// Disconnect disconnects the specified MCP server.
func (m *MCPClientManager) Disconnect(serverName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, exists := m.clients[serverName]
	if !exists {
		return fmt.Errorf("server %s not found", serverName)
	}

	if c.Client != nil {
		if err := c.Client.Close(); err != nil {
			return fmt.Errorf("failed to close client %s: %w", serverName, err)
		}
	}

	delete(m.clients, serverName)
	return nil
}

// CallTool invokes a tool on the specified MCP server.
func (m *MCPClientManager) CallTool(
	ctx context.Context,
	serverName string,
	toolName string,
	arguments map[string]interface{},
) (*mcp.CallToolResult, error) {
	m.mu.RLock()
	c, exists := m.clients[serverName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("server %s not connected", serverName)
	}

	if !c.Connected || c.Client == nil {
		return nil, fmt.Errorf("server %s is not in connected state", serverName)
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = arguments

	return c.Client.CallTool(ctx, req)
}

// ListTools lists available tools on the specified MCP server.
func (m *MCPClientManager) ListTools(ctx context.Context, serverName string) (*mcp.ListToolsResult, error) {
	m.mu.RLock()
	c, exists := m.clients[serverName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("server %s not connected", serverName)
	}

	if !c.Connected || c.Client == nil {
		return nil, fmt.Errorf("server %s is not in connected state", serverName)
	}

	req := mcp.ListToolsRequest{}
	return c.Client.ListTools(ctx, req)
}

// GetClient retrieves an MCP client by name.
func (m *MCPClientManager) GetClient(serverName string) (*MCPClient, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, exists := m.clients[serverName]
	return c, exists
}

// ListConnectedServers returns all connected server names.
func (m *MCPClientManager) ListConnectedServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}

// Close closes all MCP connections.
func (m *MCPClientManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for name, c := range m.clients {
		if c.Client != nil {
			if err := c.Client.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close %s: %w", name, err))
			}
		}
	}
	m.clients = make(map[string]*MCPClient)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing clients: %v", errs)
	}
	return nil
}
