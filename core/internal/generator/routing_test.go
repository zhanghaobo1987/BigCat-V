package generator

import (
	"os"
	"path/filepath"
	"testing"

	"bigcatv/internal/model"
)

func testRoutingCfg() *model.RoutingConfig {
	return &model.RoutingConfig{
		DefaultAction: model.ActionDirect,
		Rules: []model.RoutingRule{
			{
				ID: "r1", Name: "广告拦截", Enabled: true, Action: model.ActionBlock,
				GeoSite: []string{"category-ads-all"},
			},
			{
				ID: "r2", Name: "CN直连", Enabled: true, Action: model.ActionDirect,
				DomainSuffix: []string{"example.cn"}, IPCIDR: []string{"1.2.3.0/24"},
				GeoIP: []string{"cn"}, Ports: []int{80, 443},
			},
			{
				ID: "r3", Name: "禁用规则", Enabled: false, Action: model.ActionBlock,
				DomainSuffix: []string{"never.example"},
			},
		},
	}
}

// 准备一个带部分 .srs 文件的 geo 目录：ads 的有，cn 的没有。
func setupGeoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "geosite-category-ads-all.srs"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSingboxRouting(t *testing.T) {
	geoDir := setupGeoDir(t)
	ruleSets, rules := singboxRouting(testRoutingCfg(), geoDir)
	if len(ruleSets) != 1 {
		t.Fatalf("期望 1 个 rule_set，得到 %d", len(ruleSets))
	}
	if tag := ruleSets[0].(map[string]any)["tag"]; tag != "geosite-category-ads-all" {
		t.Fatalf("rule_set tag 错误: %v", tag)
	}
	if len(rules) != 2 {
		t.Fatalf("期望 2 条规则（禁用跳过），得到 %d", len(rules))
	}
	r0 := rules[0].(map[string]any)
	if r0["outbound"] != "block" {
		t.Fatalf("r1 应为 block，得到 %v", r0["outbound"])
	}
	if _, ok := r0["rule_set"]; !ok {
		t.Fatal("r1 应引用 rule_set")
	}
	// r2：geoip-cn.srs 缺失，geoip 匹配被丢弃，但域名/IP/端口保留
	r1 := rules[1].(map[string]any)
	if r1["outbound"] != "direct" {
		t.Fatalf("r2 应为 direct，得到 %v", r1["outbound"])
	}
	if _, ok := r1["domain_suffix"]; !ok {
		t.Fatal("r2 应保留 domain_suffix")
	}
	if _, ok := r1["ip_cidr"]; !ok {
		t.Fatal("r2 应保留 ip_cidr")
	}
	if _, ok := r1["rule_set"]; ok {
		t.Fatal("r2 的 geoip-cn.srs 缺失，不应引用 rule_set")
	}
}

func TestSingboxGenerateWithRouting(t *testing.T) {
	geoDir := setupGeoDir(t)
	cfg, err := Generate(nil, Options{Routing: testRoutingCfg(), GeoDir: geoDir})
	if err != nil {
		t.Fatal(err)
	}
	route := cfg["route"].(map[string]any)
	if route["final"] != "direct" {
		t.Fatalf("final 应为 direct，得到 %v", route["final"])
	}
	rules := route["rules"].([]any)
	// dns + 2 自定义 + 2 内置 = 5
	if len(rules) != 5 {
		t.Fatalf("期望 5 条 route rules，得到 %d", len(rules))
	}
	if _, ok := route["rule_set"]; !ok {
		t.Fatal("应包含 rule_set")
	}
}

func TestXrayRouting(t *testing.T) {
	// 无 .dat：r1（纯 geosite）整条跳过，r2 保留非 geo 条件
	rules := xrayRouting(testRoutingCfg(), t.TempDir())
	if len(rules) != 1 {
		t.Fatalf("期望 1 条规则，得到 %d", len(rules))
	}
	r0 := rules[0].(map[string]any)
	if r0["outboundTag"] != "direct" {
		t.Fatalf("应为 direct，得到 %v", r0["outboundTag"])
	}
	if _, ok := r0["domain"]; !ok {
		t.Fatal("r2 应保留 domain_suffix")
	}
}

func TestXrayRoutingWithDat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "geoip.dat"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "geosite.dat"), []byte("x"), 0644)
	rules := xrayRouting(testRoutingCfg(), dir)
	if len(rules) != 2 {
		t.Fatalf("期望 2 条规则，得到 %d", len(rules))
	}
	r0 := rules[0].(map[string]any)
	doms := r0["domain"].([]string)
	if len(doms) != 1 || doms[0] != "geosite:category-ads-all" {
		t.Fatalf("r1 domain 错误: %v", doms)
	}
	r1 := rules[1].(map[string]any)
	ips := r1["ip"].([]string)
	found := false
	for _, ip := range ips {
		if ip == "geoip:cn" {
			found = true
		}
	}
	if !found {
		t.Fatalf("r2 ip 应包含 geoip:cn: %v", ips)
	}
	if r1["port"] != "80,443" {
		t.Fatalf("r1 port 错误: %v", r1["port"])
	}
}

func TestXrayGenerateWithRouting(t *testing.T) {
	dir := t.TempDir()
	cfg, err := GenerateXray(nil, Options{Routing: testRoutingCfg(), GeoDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	rules := cfg["routing"].(map[string]any)["rules"].([]any)
	// 无 .dat：1 自定义(r2) + 2 内置 + 1 兜底(direct) = 4
	if len(rules) != 4 {
		t.Fatalf("期望 4 条 routing rules，得到 %d", len(rules))
	}
	last := rules[len(rules)-1].(map[string]any)
	if last["outboundTag"] != "direct" {
		t.Fatalf("兜底规则应为 direct，得到 %v", last["outboundTag"])
	}
}

func TestRoutingRuleValidate(t *testing.T) {
	bad := []model.RoutingRule{
		{Name: "", Action: model.ActionProxy, DomainSuffix: []string{"a.com"}},                       // 空名
		{Name: "x", Action: "fly", DomainSuffix: []string{"a.com"}},                                  // 非法动作
		{Name: "x", Action: model.ActionProxy},                                                       // 无条件
		{Name: "x", Action: model.ActionProxy, IPCIDR: []string{"999.1.1.1"}},                        // 非法 CIDR
		{Name: "x", Action: model.ActionProxy, DomainRegex: []string{"(["}},                          // 非法正则
		{Name: "x", Action: model.ActionProxy, Ports: []int{99999}, DomainSuffix: []string{"a.com"}}, // 非法端口
		{Name: "x", Action: model.ActionProxy, GeoIP: []string{"CN!"}, DomainSuffix: []string{"a.com"}},
	}
	for i, r := range bad {
		if err := r.Validate(); err == nil {
			t.Fatalf("用例 %d 应校验失败", i)
		}
	}
	ok := model.RoutingRule{Name: "ok", Action: model.ActionDirect, IPCIDR: []string{"10.0.0.1", "1.2.3.0/24"}, GeoIP: []string{"cn"}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("合法规则不应失败: %v", err)
	}
}
