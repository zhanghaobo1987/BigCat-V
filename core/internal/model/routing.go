package model

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// RoutingAction 分流动作。
type RoutingAction string

const (
	ActionProxy  RoutingAction = "proxy"  // 走代理
	ActionDirect RoutingAction = "direct" // 直连
	ActionBlock  RoutingAction = "block"  // 拦截
)

// RoutingRule 单条分流规则。规则按列表顺序匹配，首条命中生效。
//
// 同一规则内的各匹配条件为"或"关系（任一命中即整条规则命中）。
type RoutingRule struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Enabled bool          `json:"enabled"`
	Action  RoutingAction `json:"action"`

	// 匹配条件（均为可选，至少填一项）
	DomainSuffix  []string `json:"domain_suffix,omitempty"`  // 域名后缀，如 example.com
	DomainKeyword []string `json:"domain_keyword,omitempty"` // 域名关键字
	DomainRegex   []string `json:"domain_regex,omitempty"`   // 域名正则
	IPCIDR        []string `json:"ip_cidr,omitempty"`        // CIDR，如 1.2.3.0/24
	GeoIP         []string `json:"geoip,omitempty"`          // 国家/地区代码，如 cn
	GeoSite       []string `json:"geosite,omitempty"`        // 域名分类，如 cn / category-ads-all
	Ports         []int    `json:"port,omitempty"`           // 目标端口
	ProcessNames  []string `json:"process,omitempty"`        // 进程名（仅 sing-box 生效）
}

// RoutingConfig 全局分流配置。
type RoutingConfig struct {
	Rules         []RoutingRule `json:"rules"`
	DefaultAction RoutingAction `json:"default_action"` // 未命中任何规则时的动作
}

// HasMatchers 是否至少有一个匹配条件。
func (r *RoutingRule) HasMatchers() bool {
	return len(r.DomainSuffix) > 0 || len(r.DomainKeyword) > 0 ||
		len(r.DomainRegex) > 0 || len(r.IPCIDR) > 0 ||
		len(r.GeoIP) > 0 || len(r.GeoSite) > 0 ||
		len(r.Ports) > 0 || len(r.ProcessNames) > 0
}

// Validate 校验规则合法性。
func (r *RoutingRule) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("规则名称为空")
	}
	switch r.Action {
	case ActionProxy, ActionDirect, ActionBlock:
	default:
		return fmt.Errorf("未知动作 %q", r.Action)
	}
	if !r.HasMatchers() {
		return fmt.Errorf("规则 %q 至少需要一个匹配条件", r.Name)
	}
	for _, c := range r.IPCIDR {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(c)); err != nil {
			if ip := net.ParseIP(strings.TrimSpace(c)); ip == nil {
				return fmt.Errorf("规则 %q 的 ip_cidr %q 非法", r.Name, c)
			}
		}
	}
	for _, re := range r.DomainRegex {
		if _, err := regexp.Compile(re); err != nil {
			return fmt.Errorf("规则 %q 的 domain_regex %q 非法: %v", r.Name, re, err)
		}
	}
	for _, p := range r.Ports {
		if p <= 0 || p > 65535 {
			return fmt.Errorf("规则 %q 的端口 %d 非法", r.Name, p)
		}
	}
	for _, g := range r.GeoIP {
		if !isGeoCode(g) {
			return fmt.Errorf("规则 %q 的 geoip %q 非法（应为小写地区代码）", r.Name, g)
		}
	}
	return nil
}

func isGeoCode(s string) bool {
	if len(s) < 2 || len(s) > 16 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

// GeoSets 收集规则引用的 geo 数据集标签（去重）。
// sing-box 侧映射为 rule_set tag：geoip-cn / geosite-cn 等。
func (r *RoutingRule) GeoSets() (geoip, geosite []string) {
	seen := map[string]bool{}
	for _, g := range r.GeoIP {
		tag := "geoip-" + strings.ToLower(g)
		if !seen[tag] {
			seen[tag] = true
			geoip = append(geoip, tag)
		}
	}
	for _, g := range r.GeoSite {
		tag := "geosite-" + strings.ToLower(g)
		if !seen[tag] {
			seen[tag] = true
			geosite = append(geosite, tag)
		}
	}
	return geoip, geosite
}

// Summary 匹配条件摘要（供 UI 列表展示）。
func (r *RoutingRule) Summary() string {
	var parts []string
	if len(r.DomainSuffix) > 0 {
		parts = append(parts, fmt.Sprintf("域名后缀×%d", len(r.DomainSuffix)))
	}
	if len(r.DomainKeyword) > 0 {
		parts = append(parts, fmt.Sprintf("关键字×%d", len(r.DomainKeyword)))
	}
	if len(r.DomainRegex) > 0 {
		parts = append(parts, fmt.Sprintf("正则×%d", len(r.DomainRegex)))
	}
	if len(r.IPCIDR) > 0 {
		parts = append(parts, fmt.Sprintf("IP段×%d", len(r.IPCIDR)))
	}
	if len(r.GeoIP) > 0 {
		parts = append(parts, "GeoIP:"+strings.Join(r.GeoIP, ","))
	}
	if len(r.GeoSite) > 0 {
		parts = append(parts, "GeoSite:"+strings.Join(r.GeoSite, ","))
	}
	if len(r.Ports) > 0 {
		parts = append(parts, fmt.Sprintf("端口×%d", len(r.Ports)))
	}
	if len(r.ProcessNames) > 0 {
		parts = append(parts, fmt.Sprintf("进程×%d", len(r.ProcessNames)))
	}
	if len(parts) == 0 {
		return "无条件"
	}
	return strings.Join(parts, "｜")
}
