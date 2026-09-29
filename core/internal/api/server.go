// Package api 本地控制 API：REST（127.0.0.1）+ SSE 事件流。
// 这是 bigcatvd 对外唯一的界面，各端 UI（Tauri / iOS / Android）只调这里。
//
// 双内核：Server 持有 map[string]engine.Core（"sing-box"/"xray"），
// 启动时通过 kernel 参数选择；缺失二进制的内核标记为不可用。
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"bigcatv/internal/engine"
	"bigcatv/internal/generator"
	"bigcatv/internal/geo"
	"bigcatv/internal/model"
	"bigcatv/internal/routing"
	"bigcatv/internal/subscription"
)

// Store 内存状态存储（v0.1；持久化放到 v0.2）。
type Store struct {
	mu            sync.RWMutex
	nodes         map[string]*model.Node
	subscriptions map[string]*Subscription
	activeNodeID  string
	activeKernel  string
	version       string
}

// Subscription 订阅记录。
type Subscription struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Count int    `json:"count"`
}

func NewStore(version string) *Store {
	return &Store{
		nodes:         make(map[string]*model.Node),
		subscriptions: make(map[string]*Subscription),
		version:       version,
	}
}

func (s *Store) UpsertNodes(nodes []*model.Node, group string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, nd := range nodes {
		if group != "" && nd.Group == "" {
			nd.Group = group
		}
		s.nodes[nd.ID] = nd
		n++
	}
	return n
}

func (s *Store) ListNodes() []*model.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, n)
	}
	return out
}

func (s *Store) GetNode(id string) (*model.Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	return n, ok
}

func (s *Store) DeleteNode(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodes, id)
}

// Server API 服务器。
type Server struct {
	store   *Store
	cores   map[string]engine.Core
	routing *routing.Store
	geo     *geo.Updater
	mux     *http.ServeMux
	workDir string
}

// NewServer 创建 API 服务器。cores 为可用内核映射（缺失二进制的不放入）。
func NewServer(store *Store, cores map[string]engine.Core, workDir string, routing *routing.Store, geo *geo.Updater) *Server {
	s := &Server{store: store, cores: cores, mux: http.NewServeMux(), workDir: workDir, routing: routing, geo: geo}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

// activeCore 当前内核；未选择时按 sing-box -> xray 顺序取第一个可用。
func (s *Server) activeCore() engine.Core {
	s.store.mu.RLock()
	k := s.store.activeKernel
	s.store.mu.RUnlock()
	if c, ok := s.cores[k]; ok {
		return c
	}
	if c, ok := s.cores[engine.KernelSingBox]; ok {
		return c
	}
	for _, c := range s.cores {
		return c
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/v1/kernels", s.handleKernels)
	s.mux.HandleFunc("GET /api/v1/nodes", s.handleListNodes)
	s.mux.HandleFunc("POST /api/v1/nodes", s.handleAddNode)
	s.mux.HandleFunc("DELETE /api/v1/nodes/{id}", s.handleDeleteNode)
	s.mux.HandleFunc("POST /api/v1/nodes/{id}/test", s.handleTestNode)
	s.mux.HandleFunc("GET /api/v1/subscriptions", s.handleListSubs)
	s.mux.HandleFunc("POST /api/v1/subscriptions", s.handleAddSub)
	s.mux.HandleFunc("POST /api/v1/subscriptions/{id}/refresh", s.handleRefreshSub)
	s.mux.HandleFunc("POST /api/v1/engine/start", s.handleEngineStart)
	s.mux.HandleFunc("POST /api/v1/engine/stop", s.handleEngineStop)
	s.mux.HandleFunc("GET /api/v1/engine/config", s.handleEngineConfig)
	s.mux.HandleFunc("GET /api/v1/events", s.handleEvents)
	s.mux.HandleFunc("GET /api/v1/routing/rules", s.handleListRules)
	s.mux.HandleFunc("POST /api/v1/routing/rules", s.handleAddRule)
	s.mux.HandleFunc("PUT /api/v1/routing/rules/{id}", s.handleUpdateRule)
	s.mux.HandleFunc("DELETE /api/v1/routing/rules/{id}", s.handleDeleteRule)
	s.mux.HandleFunc("POST /api/v1/routing/rules/{id}/toggle", s.handleToggleRule)
	s.mux.HandleFunc("POST /api/v1/routing/rules/reorder", s.handleReorderRules)
	s.mux.HandleFunc("PUT /api/v1/routing/default-action", s.handleSetDefaultAction)
	s.mux.HandleFunc("GET /api/v1/routing/geo", s.handleGeoStatus)
	s.mux.HandleFunc("POST /api/v1/routing/geo/update", s.handleGeoUpdate)
	s.mux.HandleFunc("PUT /api/v1/routing/geo", s.handleGeoSettings)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	core := s.activeCore()
	var snap *engine.Snapshot
	var kernel string
	if core != nil {
		ss := core.Snapshot()
		snap = &ss
		kernel = core.Name()
	}
	s.store.mu.RLock()
	active := s.store.activeNodeID
	version := s.store.version
	s.store.mu.RUnlock()
	writeJSON(w, 200, map[string]any{
		"version":     version,
		"kernel":      kernel,
		"engine":      snap,
		"active_node": active,
		"node_count":  len(s.store.ListNodes()),
	})
}

// handleKernels 内核可用性列表。
func (s *Server) handleKernels(w http.ResponseWriter, r *http.Request) {
	type kinfo struct {
		Name      string `json:"name"`
		Available bool   `json:"available"`
		Binary    string `json:"binary,omitempty"`
		Version   string `json:"version,omitempty"`
	}
	out := []kinfo{}
	for _, name := range []string{engine.KernelSingBox, engine.KernelXray} {
		ki := kinfo{Name: name}
		if c, ok := s.cores[name]; ok {
			ki.Available = true
			ki.Binary = c.Binary()
			if v, err := c.Version(); err == nil {
				ki.Version = firstLine(v)
			}
		}
		out = append(out, ki)
	}
	writeJSON(w, 200, map[string]any{"kernels": out})
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	if len(s) > 80 {
		return s[:80]
	}
	return s
}

type nodeView struct {
	*model.Node
	XrayCompatible bool   `json:"xray_compatible"`
	XrayNote       string `json:"xray_note,omitempty"`
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes := s.store.ListNodes()
	views := make([]nodeView, 0, len(nodes))
	for _, n := range nodes {
		v := nodeView{Node: n, XrayCompatible: generator.XraySupported(n.Protocol)}
		if !v.XrayCompatible {
			v.XrayNote = generator.XrayUnsupportedReason(n.Protocol)
		}
		views = append(views, v)
	}
	writeJSON(w, 200, map[string]any{"nodes": views})
}

func (s *Server) handleAddNode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Link == "" {
		writeErr(w, 400, "需要 link 字段（分享链接）")
		return
	}
	n, err := subscription.ParseShareLink(body.Link)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.store.UpsertNodes([]*model.Node{n}, "")
	writeJSON(w, 201, n)
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.store.DeleteNode(id)
	writeJSON(w, 200, map[string]string{"deleted": id})
}

func (s *Server) handleTestNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, ok := s.store.GetNode(id)
	if !ok {
		writeErr(w, 404, "节点不存在")
		return
	}
	ms, err := TCPHandshakeLatency(n.Server, n.Port)
	if err != nil {
		n.LatencyMs = -1
	} else {
		n.LatencyMs = ms
	}
	writeJSON(w, 200, map[string]any{"id": id, "latency_ms": n.LatencyMs})
}

func (s *Server) handleListSubs(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	out := make([]*Subscription, 0, len(s.store.subscriptions))
	for _, sub := range s.store.subscriptions {
		out = append(out, sub)
	}
	writeJSON(w, 200, map[string]any{"subscriptions": out})
}

func (s *Server) handleAddSub(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeErr(w, 400, "需要 url 字段")
		return
	}
	nodes, err := subscription.Fetch(body.URL, subscription.DefaultFetchOptions())
	if err != nil {
		writeErr(w, 502, "订阅抓取失败: "+err.Error())
		return
	}
	name := body.Name
	if name == "" {
		name = body.URL
	}
	sub := &Subscription{ID: "sub-" + hashShort(body.URL), Name: name, URL: body.URL, Count: len(nodes)}
	s.store.mu.Lock()
	s.store.subscriptions[sub.ID] = sub
	s.store.mu.Unlock()
	added := s.store.UpsertNodes(nodes, name)
	writeJSON(w, 201, map[string]any{"subscription": sub, "added": added})
}

func (s *Server) handleRefreshSub(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.store.mu.RLock()
	sub, ok := s.store.subscriptions[id]
	s.store.mu.RUnlock()
	if !ok {
		writeErr(w, 404, "订阅不存在")
		return
	}
	nodes, err := subscription.Fetch(sub.URL, subscription.DefaultFetchOptions())
	if err != nil {
		writeErr(w, 502, "刷新失败: "+err.Error())
		return
	}
	added := s.store.UpsertNodes(nodes, sub.Name)
	s.store.mu.Lock()
	sub.Count = len(nodes)
	s.store.mu.Unlock()
	writeJSON(w, 200, map[string]any{"refreshed": id, "added": added})
}

func (s *Server) handleEngineStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeID    string `json:"node_id"`
		Kernel    string `json:"kernel"` // "sing-box" | "xray"，空=保持当前
		Tun       bool   `json:"tun"`
		MixedPort int    `json:"mixed_port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求体非法")
		return
	}
	var core engine.Core
	if body.Kernel != "" {
		c, ok := s.cores[body.Kernel]
		if !ok {
			writeErr(w, 400, fmt.Sprintf("内核 %q 不可用（未找到二进制）", body.Kernel))
			return
		}
		core = c
	} else {
		core = s.activeCore()
	}
	if core == nil {
		writeErr(w, 500, "没有可用内核（sing-box / xray 二进制均未找到）")
		return
	}

	nodes := s.store.ListNodes()
	if body.NodeID != "" {
		n, ok := s.store.GetNode(body.NodeID)
		if !ok {
			writeErr(w, 404, "节点不存在")
			return
		}
		nodes = []*model.Node{n}
	}
	if len(nodes) == 0 {
		writeErr(w, 400, "没有可用节点")
		return
	}

	// 按内核过滤不兼容节点
	var use []*model.Node
	var skipped []string
	for _, n := range nodes {
		if core.Name() == engine.KernelXray && !generator.XraySupported(n.Protocol) {
			skipped = append(skipped, n.DisplayName())
			continue
		}
		use = append(use, n)
	}
	if len(use) == 0 {
		writeErr(w, 400, fmt.Sprintf("所选节点均不兼容 %s 内核: %v", core.Name(), skipped))
		return
	}

	var data []byte
	var err error
	opts := generator.Options{Tun: body.Tun, DefaultOutbound: orDefault(body.NodeID, "proxy")}
	if body.MixedPort >= 1024 && body.MixedPort <= 65535 {
		opts.MixedPort = body.MixedPort
	}
	// 自定义分流：规则与 geo 资源目录注入生成器
	if s.routing != nil {
		rc := s.routing.Get()
		opts.Routing = &rc
	}
	if s.geo != nil {
		opts.GeoDir = s.geo.Dir()
	}
	if core.Name() == engine.KernelXray {
		data, err = generator.MarshalXray(use, opts)
	} else {
		data, err = generator.Marshal(use, opts)
	}
	if err != nil {
		writeErr(w, 500, "配置生成失败: "+err.Error())
		return
	}
	cfgPath := filepath.Join(s.workDir, core.Name()+".json")
	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		writeErr(w, 500, "写配置失败: "+err.Error())
		return
	}
	// 切换内核时先停旧内核
	if prev := s.activeCore(); prev != nil && prev.Name() != core.Name() {
		_ = prev.Stop()
	}
	if err := core.Reload(cfgPath); err != nil {
		writeErr(w, 500, "引擎启动失败: "+err.Error())
		return
	}
	s.store.mu.Lock()
	s.store.activeNodeID = body.NodeID
	s.store.activeKernel = core.Name()
	s.store.mu.Unlock()
	writeJSON(w, 200, map[string]any{
		"started": true, "kernel": core.Name(), "config": cfgPath,
		"nodes": len(use), "skipped": skipped,
	})
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func (s *Server) handleEngineStop(w http.ResponseWriter, r *http.Request) {
	if core := s.activeCore(); core != nil {
		_ = core.Stop()
	}
	s.store.mu.Lock()
	s.store.activeNodeID = ""
	s.store.mu.Unlock()
	writeJSON(w, 200, map[string]bool{"stopped": true})
}

func (s *Server) handleEngineConfig(w http.ResponseWriter, r *http.Request) {
	core := s.activeCore()
	if core == nil {
		writeErr(w, 404, "无可用内核")
		return
	}
	data, err := os.ReadFile(filepath.Join(s.workDir, core.Name()+".json"))
	if err != nil {
		writeErr(w, 404, "暂无已生成的配置")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}

// handleEvents SSE 事件流（v0.1 用 SSE 代替 WS，保持零外部依赖）。
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "不支持流式响应")
		return
	}
	core := s.activeCore()
	if core == nil {
		writeErr(w, 500, "无可用内核")
		return
	}
	logCh := core.Logs()
	emit := func(ev, data string) {
		_, _ = w.Write([]byte("event: " + ev + "\ndata: " + data + "\n\n"))
		flusher.Flush()
	}
	emit("hello", `{"ok":true}`)
	for {
		select {
		case <-r.Context().Done():
			return
		case line := <-logCh:
			b, _ := json.Marshal(map[string]string{"line": line})
			emit("log", string(b))
		}
	}
}
