package api

import (
	"context"
	"encoding/json"
	"net/http"

	"bigcatv/internal/model"
)

// GET /api/v1/routing/rules
func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	cfg := s.routing.Get()
	writeJSON(w, 200, cfg)
}

// POST /api/v1/routing/rules
func (s *Server) handleAddRule(w http.ResponseWriter, r *http.Request) {
	var rule model.RoutingRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeErr(w, 400, "请求体非法")
		return
	}
	created, err := s.routing.Add(rule)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, created)
}

// PUT /api/v1/routing/rules/{id}
func (s *Server) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	var rule model.RoutingRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeErr(w, 400, "请求体非法")
		return
	}
	if err := s.routing.Update(r.PathValue("id"), rule); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/v1/routing/rules/{id}
func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	if err := s.routing.Delete(r.PathValue("id")); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/v1/routing/rules/{id}/toggle  body: {"enabled": bool}
func (s *Server) handleToggleRule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求体非法")
		return
	}
	if err := s.routing.SetEnabled(r.PathValue("id"), body.Enabled); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "enabled": body.Enabled})
}

// POST /api/v1/routing/rules/reorder  body: {"ids": [...]}
func (s *Server) handleReorderRules(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求体非法")
		return
	}
	if err := s.routing.Reorder(body.IDs); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// PUT /api/v1/routing/default-action  body: {"default_action": "proxy"|"direct"|"block"}
func (s *Server) handleSetDefaultAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DefaultAction model.RoutingAction `json:"default_action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求体非法")
		return
	}
	if err := s.routing.SetDefaultAction(body.DefaultAction); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "default_action": body.DefaultAction})
}

// GET /api/v1/routing/geo
func (s *Server) handleGeoStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.geo.Status())
}

// POST /api/v1/routing/geo/update  body: {"only_missing": bool}（可选，默认全量）
func (s *Server) handleGeoUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OnlyMissing bool `json:"only_missing"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // 空 body 视为全量
	// 下载可能耗时，放后台执行，立即返回 accepted（用独立 context，避免随请求取消）
	go func() {
		_ = s.geo.Update(context.Background(), body.OnlyMissing)
	}()
	writeJSON(w, 202, map[string]any{"accepted": true})
}

// PUT /api/v1/routing/geo  body: {"auto_update": bool, "interval_hours": int}
func (s *Server) handleGeoSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AutoUpdate    *bool `json:"auto_update"`
		IntervalHours int   `json:"interval_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求体非法")
		return
	}
	st := s.geo.Status()
	on := st.AutoUpdate
	if body.AutoUpdate != nil {
		on = *body.AutoUpdate
	}
	if err := s.geo.SetAutoUpdate(on, body.IntervalHours); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, s.geo.Status())
}
