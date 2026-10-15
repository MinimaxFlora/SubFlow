package api

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/subflow-org/subflow/internal/version"
)

// envPayload mirrors the environment descriptor the Sub-Store frontend reads:
// backend name and version for the connection check, plus feature flags and
// the meta.node.env block that gates Docker-specific UI.
type envPayload struct {
	Backend       string          `json:"backend"`
	Version       string          `json:"version"`
	HasNewVersion bool            `json:"hasNewVersion"`
	LatestVersion string          `json:"latestVersion"`
	Feature       map[string]bool `json:"feature"`
	Meta          envMeta         `json:"meta"`
}

type envMeta struct {
	Node envNode `json:"node"`
}

type envNode struct {
	Env map[string]string `json:"env"`
}

// handleUtilEnv reports the running backend so the frontend can verify it is
// talking to a compatible server.
func (s *Server) handleUtilEnv(w http.ResponseWriter, _ *http.Request) {
	info := version.Get()
	ver := info.Version
	if ver == "" {
		ver = "dev"
	}

	restOK(w, envPayload{
		Backend:       "SubFlow",
		Version:       ver,
		HasNewVersion: false,
		LatestVersion: ver,
		// Archive and share management are not implemented yet; hiding them
		// keeps the navigation honest.
		Feature: map[string]bool{
			"archive": false,
			"share":   false,
		},
		Meta: envMeta{Node: envNode{Env: map[string]string{
			"SUB_STORE_DOCKER": "false",
		}}},
	})
}

// handleUtilRefresh clears the cached environment so the frontend refetches it.
// SubFlow keeps no server-side env cache, so this is a successful no-op.
func (s *Server) handleUtilRefresh(w http.ResponseWriter, _ *http.Request) {
	restOK(w, map[string]any{})
}

// nodeInfoRequest is the subset of a proxy node needed to look it up.
type nodeInfoRequest struct {
	Name   string `json:"name"`
	Server string `json:"server"`
	Port   int    `json:"port"`
}

// handleNodeInfo resolves a node's server hostname. Geographic enrichment is
// not performed, so the geo fields are returned empty.
func (s *Server) handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	var req nodeInfoRequest
	if !restDecode(w, r, &req) {
		return
	}
	if req.Server == "" {
		restBadRequest(w, "MISSING_SERVER", "节点缺少 server 字段")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	resolved := req.Server
	if ips, err := net.DefaultResolver.LookupHost(ctx, req.Server); err == nil && len(ips) > 0 {
		resolved = ips[0]
	}

	restOK(w, map[string]any{
		"shareUrl": "",
		"info": map[string]any{
			"query":  resolved,
			"status": "success",
			"server": req.Server,
			"port":   req.Port,
			"name":   req.Name,
		},
	})
}
