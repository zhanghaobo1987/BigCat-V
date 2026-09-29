// Package generator 的分流规则部分：统一 RoutingRule -> sing-box / xray 路由。
//
// 语义：规则按顺序匹配、首条命中生效；同一规则内的多个匹配条件为"或"关系。
// sing-box 的 route rule 与 xray 的 field rule 在同一规则内均为 OR 语义，
// 与模型一致，可一对一翻译。
package generator

import (
	"os"
	"path/filepath"
	"strings"

	"bigcatv/internal/model"
)

// actionTag 分流动作 -> 出站 tag。
func actionTag(a model.RoutingAction) string {
	switch a {
	case model.ActionDirect:
		return "direct"
	case model.ActionBlock:
		return "block"
	default:
		return "proxy"
	}
}

// fileExists 本地文件是否存在。
func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Size() > 0
}

// singboxRouting 由分流配置生成 sing-box 的 rule_set 列表与 route rules。
// 缺失本地 .srs 文件的 geo 匹配会被丢弃；丢弃后无剩余条件的规则整条跳过。
func singboxRouting(cfg *model.RoutingConfig, geoDir string) (ruleSets []any, rules []any) {
	if cfg == nil {
		return nil, nil
	}
	seen := map[string]bool{}
	for _, r := range cfg.Rules {
		if !r.Enabled {
			continue
		}
		rule := map[string]any{}
		if len(r.DomainSuffix) > 0 {
			rule["domain_suffix"] = r.DomainSuffix
		}
		if len(r.DomainKeyword) > 0 {
			rule["domain_keyword"] = r.DomainKeyword
		}
		if len(r.DomainRegex) > 0 {
			rule["domain_regex"] = r.DomainRegex
		}
		if len(r.IPCIDR) > 0 {
			rule["ip_cidr"] = r.IPCIDR
		}
		if len(r.Ports) > 0 {
			rule["port"] = r.Ports
		}
		if len(r.ProcessNames) > 0 {
			rule["process_name"] = r.ProcessNames
		}
		var sets []string
		geoip, geosite := r.GeoSets()
		for _, tag := range append(append([]string{}, geoip...), geosite...) {
			if !fileExists(filepath.Join(geoDir, tag+".srs")) {
				continue // 本地库缺失：丢弃该匹配，不让整条规则失效
			}
			sets = append(sets, tag)
			if !seen[tag] {
				seen[tag] = true
				ruleSets = append(ruleSets, map[string]any{
					"type": "local", "format": "binary",
					"tag": tag, "path": filepath.Join(geoDir, tag+".srs"),
				})
			}
		}
		if len(sets) > 0 {
			rule["rule_set"] = sets
		}
		if len(rule) == 0 {
			continue
		}
		rule["outbound"] = actionTag(r.Action)
		rules = append(rules, rule)
	}
	return ruleSets, rules
}

// xrayRouting 由分流配置生成 xray 的 routing rules。
// 注意：xray 不支持进程名匹配，该条件会被丢弃并在文档中说明；
// geoip/geosite 匹配需要 geoDir 下存在 geoip.dat / geosite.dat，缺失时丢弃。
func xrayRouting(cfg *model.RoutingConfig, geoDir string) []any {
	if cfg == nil {
		return nil
	}
	hasDat := fileExists(filepath.Join(geoDir, "geoip.dat")) &&
		fileExists(filepath.Join(geoDir, "geosite.dat"))
	var rules []any
	for _, r := range cfg.Rules {
		if !r.Enabled {
			continue
		}
		rule := map[string]any{"type": "field"}
		var domains []string
		domains = append(domains, r.DomainSuffix...)
		for _, kw := range r.DomainKeyword {
			domains = append(domains, "keyword:"+kw)
		}
		for _, re := range r.DomainRegex {
			domains = append(domains, "regexp:"+re)
		}
		if hasDat {
			for _, g := range r.GeoSite {
				domains = append(domains, "geosite:"+strings.ToLower(g))
			}
		}
		if len(domains) > 0 {
			rule["domain"] = domains
		}
		var ips []string
		ips = append(ips, r.IPCIDR...)
		if hasDat {
			for _, g := range r.GeoIP {
				ips = append(ips, "geoip:"+strings.ToLower(g))
			}
		}
		if len(ips) > 0 {
			rule["ip"] = ips
		}
		if len(r.Ports) > 0 {
			rule["port"] = portsToString(r.Ports)
		}
		// process 匹配 xray 不支持：丢弃（见 docs 说明）
		if len(rule) == 1 { // 只有 type，无任何条件
			continue
		}
		rule["outboundTag"] = actionTag(r.Action)
		rules = append(rules, rule)
	}
	return rules
}

func portsToString(ports []int) string {
	var parts []string
	for _, p := range ports {
		parts = append(parts, itoa(p))
	}
	return strings.Join(parts, ",")
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [8]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
