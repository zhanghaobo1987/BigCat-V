// Package routing 分流规则的持久化存储（workdir/routing.json）。
package routing

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bigcatv/internal/model"
)

// Store 线程安全的规则存储。
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  model.RoutingConfig
}

// DefaultConfig 内置默认分流：CN 直连、广告拦截，其余走代理。
func DefaultConfig() model.RoutingConfig {
	return model.RoutingConfig{
		DefaultAction: model.ActionProxy,
		Rules: []model.RoutingRule{
			{
				ID: "builtin-cn-direct", Name: "大陆直连", Enabled: true,
				Action: model.ActionDirect,
				GeoIP:  []string{"cn"}, GeoSite: []string{"cn"},
			},
			{
				ID: "builtin-ads-block", Name: "广告拦截", Enabled: true,
				Action:  model.ActionBlock,
				GeoSite: []string{"category-ads-all"},
			},
		},
	}
}

// NewStore 打开（或创建）存储文件。
func NewStore(workDir string) (*Store, error) {
	s := &Store{path: filepath.Join(workDir, "routing.json")}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("读取分流配置失败: %w", err)
		}
		s.cfg = DefaultConfig()
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
		return s, nil
	}
	var cfg model.RoutingConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("分流配置解析失败: %w", err)
	}
	if cfg.DefaultAction == "" {
		cfg.DefaultAction = model.ActionProxy
	}
	s.cfg = cfg
	return s, nil
}

func (s *Store) saveLocked() error {
	s.cfg.DefaultAction = orDefault(s.cfg.DefaultAction)
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("写分流配置失败: %w", err)
	}
	return os.Rename(tmp, s.path)
}

func orDefault(a model.RoutingAction) model.RoutingAction {
	switch a {
	case model.ActionProxy, model.ActionDirect, model.ActionBlock:
		return a
	default:
		return model.ActionProxy
	}
}

// Get 返回配置副本。
func (s *Store) Get() model.RoutingConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.cfg
	out.Rules = append([]model.RoutingRule(nil), s.cfg.Rules...)
	return out
}

// SetDefaultAction 设置未命中时的默认动作。
func (s *Store) SetDefaultAction(a model.RoutingAction) error {
	a = orDefault(a)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.DefaultAction = a
	return s.saveLocked()
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "r-" + hex.EncodeToString(b[:])
}

// Add 新增规则（ID 自动生成）。
func (s *Store) Add(r model.RoutingRule) (model.RoutingRule, error) {
	r.ID = newID()
	r.Name = trimName(r.Name)
	if err := r.Validate(); err != nil {
		return model.RoutingRule{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Rules = append(s.cfg.Rules, r)
	return r, s.saveLocked()
}

// Update 按 ID 更新规则（ID 不可改）。
func (s *Store) Update(id string, r model.RoutingRule) error {
	r.ID = id
	r.Name = trimName(r.Name)
	if err := r.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Rules {
		if s.cfg.Rules[i].ID == id {
			s.cfg.Rules[i] = r
			return s.saveLocked()
		}
	}
	return fmt.Errorf("规则 %q 不存在", id)
}

// Delete 删除规则。
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Rules {
		if s.cfg.Rules[i].ID == id {
			s.cfg.Rules = append(s.cfg.Rules[:i], s.cfg.Rules[i+1:]...)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("规则 %q 不存在", id)
}

// SetEnabled 启用/禁用规则。
func (s *Store) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Rules {
		if s.cfg.Rules[i].ID == id {
			s.cfg.Rules[i].Enabled = enabled
			return s.saveLocked()
		}
	}
	return fmt.Errorf("规则 %q 不存在", id)
}

// Reorder 按给定 ID 顺序重排（ID 必须与现有集合一致）。
func (s *Store) Reorder(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) != len(s.cfg.Rules) {
		return fmt.Errorf("ID 数量不匹配（期望 %d，得到 %d）", len(s.cfg.Rules), len(ids))
	}
	byID := make(map[string]model.RoutingRule, len(s.cfg.Rules))
	for _, r := range s.cfg.Rules {
		byID[r.ID] = r
	}
	out := make([]model.RoutingRule, 0, len(ids))
	for _, id := range ids {
		r, ok := byID[id]
		if !ok {
			return fmt.Errorf("未知规则 ID %q", id)
		}
		out = append(out, r)
	}
	s.cfg.Rules = out
	return s.saveLocked()
}

func trimName(n string) string {
	if len(n) > 64 {
		return n[:64]
	}
	return n
}

// EnabledRules 返回启用的规则（保持顺序），供配置生成器使用。
func (s *Store) EnabledRules() []model.RoutingRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.RoutingRule
	for _, r := range s.cfg.Rules {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out
}

// TouchedAt 占位：返回当前时间（后续可做"规则变更后提示重启引擎"）。
func TouchedAt() time.Time { return time.Now() }
