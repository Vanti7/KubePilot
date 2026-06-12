// Package mcp implements a Model Context Protocol server over stdio, exposing
// KubePilot's update findings and risk scores to AI assistants (Claude, Cursor,
// Copilot, …). It speaks JSON-RPC 2.0 with newline-delimited messages, the
// framing used by the MCP stdio transport, and depends only on the standard
// library plus the existing store.
//
// Run it with `kubepilot mcp` and point an MCP client at that command.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
)

// protocolVersion is the MCP revision this server implements.
const protocolVersion = "2024-11-05"

const serverName = "kubepilot"

// Server is a stdio MCP server backed by the KubePilot store.
type Server struct {
	store       *store.Store
	logger      *zap.Logger
	version     string
	allowWrites bool
	tools       []tool
}

// tool is one registered MCP tool.
type tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(ctx context.Context, args json.RawMessage) (string, error)
}

// NewServer builds an MCP server. When allowWrites is false, mutating tools
// (e.g. changing a finding's status) are not registered.
func NewServer(s *store.Store, version string, allowWrites bool, logger *zap.Logger) *Server {
	srv := &Server{
		store:       s,
		logger:      logger,
		version:     version,
		allowWrites: allowWrites,
	}
	srv.registerTools()
	return srv
}

// ---------------------------------------------------------------------------
// JSON-RPC plumbing
// ---------------------------------------------------------------------------

type rpcRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// ServeStdio reads JSON-RPC requests from in and writes responses to out until
// the input stream closes. It is single-threaded: one request is fully handled
// before the next is read, which keeps stdout writes ordered without locking.
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	dec := json.NewDecoder(bufio.NewReader(in))
	enc := json.NewEncoder(out)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var req rpcRequest
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			s.logger.Warn("mcp: decode request", zap.Error(err))
			return err
		}

		resp := s.handle(ctx, &req)
		if resp == nil {
			continue // notification — no response
		}
		if err := enc.Encode(resp); err != nil {
			s.logger.Warn("mcp: encode response", zap.Error(err))
			return err
		}
	}
}

func (s *Server) handle(ctx context.Context, req *rpcRequest) *rpcResponse {
	// Notifications carry no id and never get a response.
	isNotification := req.ID == nil

	switch req.Method {
	case "initialize":
		return s.reply(req, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    serverName,
				"version": s.version,
			},
		})

	case "notifications/initialized", "notifications/cancelled":
		return nil

	case "ping":
		return s.reply(req, map[string]any{})

	case "tools/list":
		return s.reply(req, map[string]any{"tools": s.toolDefs()})

	case "tools/call":
		if isNotification {
			return nil
		}
		return s.callTool(ctx, req)

	default:
		if isNotification {
			return nil
		}
		return s.fail(req, codeMethodNotFound, "method not found: "+req.Method, nil)
	}
}

func (s *Server) reply(req *rpcRequest, result any) *rpcResponse {
	if req.ID == nil {
		return nil
	}
	return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) fail(req *rpcRequest, code int, msg string, data any) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: code, Message: msg, Data: data}}
}

// ---------------------------------------------------------------------------
// Tools
// ---------------------------------------------------------------------------

func (s *Server) toolDefs() []map[string]any {
	defs := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		defs = append(defs, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		})
	}
	return defs
}

func (s *Server) callTool(ctx context.Context, req *rpcRequest) *rpcResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.fail(req, codeInvalidParams, "invalid params: "+err.Error(), nil)
	}

	for _, t := range s.tools {
		if t.Name != params.Name {
			continue
		}
		text, err := t.Handler(ctx, params.Arguments)
		if err != nil {
			// MCP convention: tool execution failures are returned as a result
			// with isError=true, not as a JSON-RPC protocol error.
			return s.reply(req, toolResult("error: "+err.Error(), true))
		}
		return s.reply(req, toolResult(text, false))
	}
	return s.fail(req, codeMethodNotFound, "unknown tool: "+params.Name, nil)
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

// schema is a small helper to build a JSON Schema object for tool inputs.
func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func strProp(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func intProp(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }

func (s *Server) registerTools() {
	s.tools = []tool{
		{
			Name:        "list_clusters",
			Description: "List all Kubernetes clusters registered in KubePilot, with their environment and status.",
			InputSchema: schema(map[string]any{}),
			Handler:     s.handleListClusters,
		},
		{
			Name: "list_findings",
			Description: "List update findings (image/helm/node), ordered by risk score descending. " +
				"Filter by cluster (name or id), severity (critical/high/medium/low/info), " +
				"status (open/planned/ignored/approved/blocked/resolved), and kind (image/helm/node).",
			InputSchema: schema(map[string]any{
				"cluster":  strProp("Cluster name or UUID to filter by"),
				"severity": strProp("Severity filter"),
				"status":   strProp("Finding status filter"),
				"kind":     strProp("Finding kind filter (image/helm/node)"),
				"limit":    intProp("Max results (default 50, max 500)"),
			}),
			Handler: s.handleListFindings,
		},
		{
			Name:        "get_finding",
			Description: "Get a single update finding by its UUID, including its computed risk score and factor breakdown.",
			InputSchema: schema(map[string]any{
				"id": strProp("Finding UUID"),
			}, "id"),
			Handler: s.handleGetFinding,
		},
		{
			Name:        "findings_summary",
			Description: "Return counts of open findings grouped by severity, optionally scoped to one or more clusters.",
			InputSchema: schema(map[string]any{
				"clusters": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Cluster names or UUIDs to scope the summary (empty = all clusters)",
				},
			}),
			Handler: s.handleFindingsSummary,
		},
		{
			Name:        "top_risks",
			Description: "Return the highest-scored open findings across all clusters - the items most in need of action.",
			InputSchema: schema(map[string]any{
				"limit": intProp("Number of findings to return (default 10)"),
			}),
			Handler: s.handleTopRisks,
		},
	}

	if s.allowWrites {
		s.tools = append(s.tools, tool{
			Name: "set_finding_status",
			Description: "Change a finding's triage status. Valid statuses: " +
				"open, planned, ignored, approved, blocked, resolved.",
			InputSchema: schema(map[string]any{
				"id":     strProp("Finding UUID"),
				"status": strProp("New status"),
				"reason": strProp("Optional reason for the status change"),
			}, "id", "status"),
			Handler: s.handleSetFindingStatus,
		})
	}
}

// ---------------------------------------------------------------------------
// Tool handlers
// ---------------------------------------------------------------------------

func (s *Server) handleListClusters(ctx context.Context, _ json.RawMessage) (string, error) {
	clusters, err := s.store.ListClusters(ctx)
	if err != nil {
		return "", err
	}
	return toJSON(clusters)
}

func (s *Server) handleListFindings(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Cluster  string `json:"cluster"`
		Severity string `json:"severity"`
		Status   string `json:"status"`
		Kind     string `json:"kind"`
		Limit    int    `json:"limit"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return "", err
	}

	clusterID, err := s.resolveClusterID(ctx, in.Cluster)
	if err != nil {
		return "", err
	}

	findings, total, err := s.store.ListFindings(ctx, store.FindingFilter{
		ClusterID: clusterID,
		Severity:  in.Severity,
		Status:    in.Status,
		Kind:      in.Kind,
		Limit:     in.Limit,
	})
	if err != nil {
		return "", err
	}
	return toJSON(map[string]any{"total": total, "findings": findings})
}

func (s *Server) handleGetFinding(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		ID string `json:"id"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return "", err
	}
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}

	finding, err := s.store.GetFinding(ctx, in.ID)
	if err != nil {
		return "", err
	}
	score, _ := s.store.GetRiskScore(ctx, in.ID)
	return toJSON(map[string]any{"finding": finding, "risk_score": score})
}

func (s *Server) handleFindingsSummary(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Clusters []string `json:"clusters"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return "", err
	}

	var ids []string
	for _, ref := range in.Clusters {
		id, err := s.resolveClusterID(ctx, ref)
		if err != nil {
			return "", err
		}
		if id != "" {
			ids = append(ids, id)
		}
	}

	summary, err := s.store.GetFindingSummary(ctx, ids)
	if err != nil {
		return "", err
	}
	return toJSON(summary)
}

func (s *Server) handleTopRisks(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Limit int `json:"limit"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return "", err
	}
	if in.Limit <= 0 {
		in.Limit = 10
	}
	findings, err := s.store.TopCriticalFindings(ctx, in.Limit)
	if err != nil {
		return "", err
	}
	return toJSON(findings)
}

func (s *Server) handleSetFindingStatus(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return "", err
	}
	if in.ID == "" || in.Status == "" {
		return "", fmt.Errorf("id and status are required")
	}
	if !validStatus(in.Status) {
		return "", fmt.Errorf("invalid status %q", in.Status)
	}

	// userID is empty: the change is attributed to the MCP integration.
	if err := s.store.UpdateFindingStatus(ctx, in.ID, in.Status, ""); err != nil {
		return "", err
	}
	return toJSON(map[string]any{"id": in.ID, "status": in.Status, "updated": true})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveClusterID accepts a UUID or a cluster name/slug and returns the cluster
// UUID. An empty ref returns an empty id (meaning "all clusters").
func (s *Server) resolveClusterID(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	if _, err := uuid.Parse(ref); err == nil {
		return ref, nil
	}

	clusters, err := s.store.ListClusters(ctx)
	if err != nil {
		return "", err
	}
	lower := strings.ToLower(ref)
	for _, c := range clusters {
		if strings.ToLower(c.Name) == lower || strings.ToLower(c.Slug) == lower {
			return c.ID.String(), nil
		}
	}
	return "", fmt.Errorf("no cluster matching %q", ref)
}

func validStatus(status string) bool {
	switch status {
	case models.FindingStatusOpen,
		models.FindingStatusPlanned,
		models.FindingStatusIgnored,
		models.FindingStatusApproved,
		models.FindingStatusBlocked,
		models.FindingStatusResolved:
		return true
	default:
		return false
	}
}

func decodeArgs(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func toJSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
