// BigCat V 桌面 UI：只调 bigcatvd 本地 API，不直连任何代理内核。
"use strict";

const $ = (id) => document.getElementById(id);
const settings = {
  get apiBase() { return (localStorage.getItem("bcv_api") || "http://127.0.0.1:17890").replace(/\/$/, ""); },
  set apiBase(v) { localStorage.setItem("bcv_api", v); },
  get kernel() { return localStorage.getItem("bcv_kernel") || ""; },
  set kernel(v) { localStorage.setItem("bcv_kernel", v); },
  get tun() { return localStorage.getItem("bcv_tun") === "1"; },
  set tun(v) { localStorage.setItem("bcv_tun", v ? "1" : "0"); },
  get mixedPort() { return parseInt(localStorage.getItem("bcv_port") || "7890", 10); },
  set mixedPort(v) { localStorage.setItem("bcv_port", String(v)); },
};

const state = {
  connected: false,
  status: null,
  kernels: [],
  nodes: [],
  subs: [],
  rules: [],
  defaultAction: "proxy",
  geo: null,
  sse: null,
  logLines: 0,
};

async function api(path, opts = {}) {
  const { timeout, ...fetchOpts } = opts;
  const ctrl = new AbortController();
  const timer = timeout ? setTimeout(() => ctrl.abort(), timeout) : null;
  try {
    const res = await fetch(settings.apiBase + path, {
      headers: { "Content-Type": "application/json" },
      signal: ctrl.signal,
      ...fetchOpts,
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || ("HTTP " + res.status));
    return data;
  } catch (e) {
    if (e.name === "AbortError") throw new Error("请求超时（后端可能正在下载 geo 文件，请稍后重试）");
    throw e;
  } finally {
    if (timer) clearTimeout(timer);
  }
}

function toast(msg, ms = 2600) {
  const t = $("toast");
  t.textContent = msg;
  t.classList.remove("hidden");
  clearTimeout(t._timer);
  t._timer = setTimeout(() => t.classList.add("hidden"), ms);
}

function setConnected(ok) {
  state.connected = ok;
  $("connWarn").classList.toggle("hidden", ok);
  if (!ok) {
    $("connWarnAddr").textContent = settings.apiBase;
    $("statusPill").className = "pill off";
    $("statusPill").textContent = "未连接";
  }
}

// ---------- 状态 ----------
async function refreshStatus() {
  try {
    const s = await api("/api/v1/status");
    setConnected(true);
    state.status = s;
    const pill = $("statusPill");
    const st = s.engine ? s.engine.status : "stopped";
    if (st === "running") { pill.className = "pill run"; pill.textContent = "运行中"; }
    else if (st === "error") { pill.className = "pill err"; pill.textContent = "异常"; }
    else { pill.className = "pill off"; pill.textContent = "已停止"; }

    const kb = $("kernelBadge");
    if (s.kernel) { kb.classList.remove("hidden"); kb.textContent = s.kernel; }
    else kb.classList.add("hidden");

    const an = $("activeNodeLabel");
    if (s.active_node) {
      const n = state.nodes.find((x) => x.id === s.active_node);
      an.classList.remove("hidden");
      an.textContent = "▶ " + (n ? n.name : s.active_node);
    } else an.classList.add("hidden");

    $("aboutBox").innerHTML =
      "核心版本 " + esc(s.version) +
      (s.engine ? "<br>引擎 " + esc(s.kernel || "") + " · " + esc(s.engine.binary || "") : "") +
      (s.engine && s.engine.last_error ? '<br><span style="color:var(--red)">' + esc(s.engine.last_error) + "</span>" : "");
  } catch (e) {
    setConnected(false);
  }
}

async function refreshKernels() {
  try {
    const d = await api("/api/v1/kernels");
    state.kernels = d.kernels || [];
    const sel = $("setKernel");
    sel.innerHTML = "";
    for (const k of state.kernels) {
      const o = document.createElement("option");
      o.value = k.name;
      o.textContent = k.name + (k.available ? "" : "（不可用）");
      o.disabled = !k.available;
      sel.appendChild(o);
    }
    const avail = state.kernels.find((k) => k.available && k.name === settings.kernel)
      || state.kernels.find((k) => k.available);
    if (avail) { sel.value = avail.name; settings.kernel = avail.name; }
    const availText = state.kernels.filter((k) => k.available).map((k) => k.name).join(" / ") || "无可用内核";
    $("kernelAvail").textContent = "可用：" + availText;
  } catch (e) { /* 连接失败时保持 */ }
}

// ---------- 节点 ----------
function latencyClass(ms) {
  if (ms == null || ms === 0) return ["lat-na", "未测"];
  if (ms < 0) return ["lat-bad", "超时"];
  if (ms < 300) return ["lat-ok", ms + "ms"];
  if (ms < 800) return ["lat-mid", ms + "ms"];
  return ["lat-bad", ms + "ms"];
}

function filteredNodes() {
  const q = $("nodeSearch").value.trim().toLowerCase();
  const pf = $("protoFilter").value;
  return state.nodes.filter((n) => {
    if (pf && n.protocol !== pf) return false;
    if (!q) return true;
    return (n.name + " " + n.server + " " + (n.group || "") + " " + n.protocol).toLowerCase().includes(q);
  });
}

async function refreshNodes() {
  try {
    const d = await api("/api/v1/nodes");
    state.nodes = d.nodes || [];
    renderNodes();
    const protos = [...new Set(state.nodes.map((n) => n.protocol))].sort();
    const sel = $("protoFilter");
    const cur = sel.value;
    sel.innerHTML = '<option value="">全部协议</option>' + protos.map((p) => `<option>${esc(p)}</option>`).join("");
    sel.value = cur;
  } catch (e) { /* 忽略，状态轮询会提示 */ }
}

function renderNodes() {
  const rows = filteredNodes();
  const activeId = state.status && state.status.active_node;
  $("nodeCount").textContent = `共 ${state.nodes.length} 个节点` + (rows.length !== state.nodes.length ? `（筛选出 ${rows.length}）` : "");
  $("nodeRows").innerHTML = rows.map((n) => {
    const [cls, txt] = latencyClass(n.latency_ms);
    const compat = n.xray_compatible
      ? '<span class="compat-yes">✓</span>'
      : `<span class="compat-no" title="${esc(n.xray_note || "")}">✗</span>`;
    return `<tr class="${n.id === activeId ? "active" : ""}">
      <td class="node-name" title="${esc(n.name)}">${esc(n.name)}${n.group ? ` <span class="muted">[${esc(n.group)}]</span>` : ""}</td>
      <td><span class="proto">${esc(n.protocol)}</span></td>
      <td class="muted">${esc(n.server)}:${n.port}</td>
      <td class="${cls}">${txt}</td>
      <td>${compat}</td>
      <td class="ops">
        <button data-act="start" data-id="${n.id}" class="primary">启动</button>
        <button data-act="test" data-id="${n.id}">测速</button>
        <button data-act="del" data-id="${n.id}" class="ghost">删除</button>
      </td>
    </tr>`;
  }).join("") || `<tr><td colspan="6" class="muted" style="text-align:center;padding:24px">暂无节点，先添加订阅或手动导入分享链接</td></tr>`;
}

$("nodeRows").addEventListener("click", async (e) => {
  const btn = e.target.closest("button[data-act]");
  if (!btn) return;
  const { act, id } = btn.dataset;
  btn.disabled = true;
  try {
    if (act === "start") await startEngine(id);
    else if (act === "test") {
      const r = await api(`/api/v1/nodes/${id}/test`, { method: "POST" });
      const n = state.nodes.find((x) => x.id === id);
      if (n) n.latency_ms = r.latency_ms;
      renderNodes();
    } else if (act === "del") {
      if (!confirm("删除该节点？")) return;
      await api(`/api/v1/nodes/${id}`, { method: "DELETE" });
      await refreshNodes();
      toast("已删除");
    }
  } catch (err) { toast("失败：" + err.message); }
  btn.disabled = false;
});

$("nodeSearch").addEventListener("input", renderNodes);
$("protoFilter").addEventListener("change", renderNodes);

$("btnAddNode").addEventListener("click", () => $("addNodeForm").classList.toggle("hidden"));
$("btnCancelAddNode").addEventListener("click", () => $("addNodeForm").classList.add("hidden"));
$("btnDoAddNode").addEventListener("click", async () => {
  const lines = $("addNodeLinks").value.split("\n").map((s) => s.trim()).filter(Boolean);
  if (!lines.length) { toast("请先粘贴分享链接"); return; }
  let ok = 0, fail = 0;
  for (const link of lines) {
    try { await api("/api/v1/nodes", { method: "POST", body: JSON.stringify({ link }) }); ok++; }
    catch (e) { fail++; }
  }
  $("addNodeResult").textContent = `导入完成：成功 ${ok}，失败 ${fail}`;
  $("addNodeLinks").value = "";
  await refreshNodes();
  toast(`导入 ${ok} 个节点`);
});

$("btnTestAll").addEventListener("click", async () => {
  const nodes = filteredNodes();
  if (!nodes.length) return;
  toast(`开始测速 ${nodes.length} 个节点…`);
  const pool = 6;
  for (let i = 0; i < nodes.length; i += pool) {
    await Promise.all(nodes.slice(i, i + pool).map(async (n) => {
      try {
        const r = await api(`/api/v1/nodes/${n.id}/test`, { method: "POST" });
        n.latency_ms = r.latency_ms;
      } catch (e) { n.latency_ms = -1; }
    }));
    renderNodes();
  }
  state.nodes.sort((a, b) => {
    const x = a.latency_ms <= 0 ? 99999 : a.latency_ms;
    const y = b.latency_ms <= 0 ? 99999 : b.latency_ms;
    return x - y;
  });
  renderNodes();
  toast("测速完成，已按延迟排序");
});

// ---------- 订阅 ----------
async function refreshSubs() {
  try {
    const d = await api("/api/v1/subscriptions");
    state.subs = d.subscriptions || [];
    $("subList").innerHTML = state.subs.map((s) => `
      <div class="card sub-card">
        <div class="grow">
          <div class="name">${esc(s.name)}</div>
          <div class="url">${esc(s.url)}</div>
          <div class="muted">${s.count} 个节点</div>
        </div>
        <button data-refresh="${s.id}">刷新</button>
      </div>`).join("") || '<div class="muted">暂无订阅</div>';
  } catch (e) { /* 忽略 */ }
}

$("subList").addEventListener("click", async (e) => {
  const btn = e.target.closest("button[data-refresh]");
  if (!btn) return;
  btn.disabled = true;
  try {
    const r = await api(`/api/v1/subscriptions/${btn.dataset.refresh}/refresh`, { method: "POST" });
    toast(`刷新完成，更新 ${r.added} 个节点`);
    await refreshSubs(); await refreshNodes();
  } catch (err) { toast("刷新失败：" + err.message); }
  btn.disabled = false;
});

$("btnAddSub").addEventListener("click", async () => {
  const name = $("subName").value.trim(), url = $("subUrl").value.trim();
  if (!url) { toast("请填写订阅 URL"); return; }
  $("addSubResult").textContent = "抓取中…";
  try {
    const r = await api("/api/v1/subscriptions", { method: "POST", body: JSON.stringify({ name, url }) });
    $("addSubResult").textContent = `成功导入 ${r.added} 个节点`;
    $("subName").value = ""; $("subUrl").value = "";
    await refreshSubs(); await refreshNodes();
  } catch (err) {
    $("addSubResult").textContent = "失败：" + err.message;
  }
});

// ---------- 分流规则 ----------
const ACTIONS = { proxy: "走代理", direct: "直连", block: "拦截" };

// 把规则的条件数组拼成摘要，如 "GeoIP:cn｜端口×2"
function ruleSummary(r) {
  const parts = [];
  const n = (a) => (a || []).length;
  if (n(r.domain_suffix)) parts.push("域名后缀×" + n(r.domain_suffix));
  if (n(r.domain_keyword)) parts.push("关键字×" + n(r.domain_keyword));
  if (n(r.domain_regex)) parts.push("正则×" + n(r.domain_regex));
  if (n(r.ip_cidr)) parts.push("IP×" + n(r.ip_cidr));
  if (n(r.geoip)) parts.push("GeoIP:" + r.geoip.join(","));
  if (n(r.geosite)) parts.push("GeoSite:" + r.geosite.join(","));
  if (n(r.port)) parts.push("端口×" + n(r.port));
  if (n(r.process)) parts.push("进程×" + n(r.process));
  return parts.join("｜") || "无条件（全匹配）";
}

async function refreshRules() {
  try {
    const d = await api("/api/v1/routing/rules");
    state.rules = d.rules || [];
    state.defaultAction = d.default_action || "proxy";
    $("defaultAction").value = state.defaultAction;
    renderRules();
  } catch (e) { /* 忽略，状态轮询会提示 */ }
}

function renderRules() {
  const rows = state.rules;
  $("ruleRows").innerHTML = rows.map((r, i) => `
    <tr class="${r.enabled ? "" : "disabled-row"}">
      <td class="node-name" title="${esc(r.name)}">${esc(r.name)}</td>
      <td><span class="abadge ${esc(r.action)}">${ACTIONS[r.action] || esc(r.action)}</span></td>
      <td class="rule-cond">${esc(ruleSummary(r))}</td>
      <td><label class="switch"><input type="checkbox" data-toggle="${esc(r.id)}" ${r.enabled ? "checked" : ""}><span class="slider"></span></label></td>
      <td class="ops">
        <button data-ract="up" data-id="${esc(r.id)}" ${i === 0 ? "disabled" : ""}>↑</button>
        <button data-ract="down" data-id="${esc(r.id)}" ${i === rows.length - 1 ? "disabled" : ""}>↓</button>
      </td>
      <td class="ops">
        <button data-ract="edit" data-id="${esc(r.id)}">编辑</button>
        <button data-ract="del" data-id="${esc(r.id)}" class="ghost">删除</button>
      </td>
    </tr>`).join("") || `<tr><td colspan="6" class="muted" style="text-align:center;padding:24px">暂无规则，所有流量走默认动作</td></tr>`;
}

$("ruleRows").addEventListener("click", async (e) => {
  const btn = e.target.closest("button[data-ract]");
  if (!btn || btn.disabled) return;
  const { ract, id } = btn.dataset;
  if (ract === "edit") { openRuleModal(state.rules.find((r) => String(r.id) === id)); return; }
  btn.disabled = true;
  try {
    if (ract === "del") {
      if (!confirm("删除该规则？")) { btn.disabled = false; return; }
      await api(`/api/v1/routing/rules/${encodeURIComponent(id)}`, { method: "DELETE" });
      toast("已删除");
    } else if (ract === "up" || ract === "down") {
      const ids = state.rules.map((r) => r.id);
      const i = ids.findIndex((x) => String(x) === id);
      const j = ract === "up" ? i - 1 : i + 1;
      if (i < 0 || j < 0 || j >= ids.length) { btn.disabled = false; return; }
      [ids[i], ids[j]] = [ids[j], ids[i]];
      await api("/api/v1/routing/rules/reorder", { method: "POST", body: JSON.stringify({ ids }) });
    }
    await refreshRules();
  } catch (err) { toast("失败：" + err.message); }
  btn.disabled = false;
});

// 启用开关即时切换
$("ruleRows").addEventListener("change", async (e) => {
  const t = e.target.closest("input[data-toggle]");
  if (!t) return;
  const id = t.dataset.toggle;
  try {
    await api(`/api/v1/routing/rules/${encodeURIComponent(id)}/toggle`, {
      method: "POST", body: JSON.stringify({ enabled: t.checked }),
    });
    const r = state.rules.find((x) => String(x.id) === id);
    if (r) r.enabled = t.checked;
    renderRules();
  } catch (err) { toast("失败：" + err.message); t.checked = !t.checked; }
});

// 未命中默认动作
$("defaultAction").addEventListener("change", async () => {
  try {
    await api("/api/v1/routing/default-action", {
      method: "PUT", body: JSON.stringify({ default_action: $("defaultAction").value }),
    });
    state.defaultAction = $("defaultAction").value;
    toast("默认动作已更新");
  } catch (err) { toast("失败：" + err.message); await refreshRules(); }
});

// ---------- 规则编辑弹窗 ----------
let editingRuleId = null;

function openRuleModal(r) {
  editingRuleId = r ? r.id : null;
  $("ruleModalTitle").textContent = r ? "编辑规则" : "新增规则";
  $("ruleName").value = r ? r.name : "";
  $("ruleAction").value = r ? r.action : "proxy";
  $("ruleEnabled").checked = r ? !!r.enabled : true;
  const lines = (a) => (a || []).join("\n");
  const csv = (a) => (a || []).join(",");
  $("ruleDomainSuffix").value = lines(r && r.domain_suffix);
  $("ruleDomainKeyword").value = lines(r && r.domain_keyword);
  $("ruleDomainRegex").value = lines(r && r.domain_regex);
  $("ruleIpCidr").value = lines(r && r.ip_cidr);
  $("ruleGeoip").value = csv(r && r.geoip);
  $("ruleGeosite").value = csv(r && r.geosite);
  $("rulePort").value = csv(r && r.port);
  $("ruleProcess").value = csv(r && r.process);
  $("ruleModal").classList.remove("hidden");
}

function closeRuleModal() { $("ruleModal").classList.add("hidden"); }

$("btnAddRule").addEventListener("click", () => openRuleModal(null));
$("btnCloseRuleModal").addEventListener("click", closeRuleModal);
$("btnCancelRule").addEventListener("click", closeRuleModal);
$("ruleModal").addEventListener("click", (e) => { if (e.target.id === "ruleModal") closeRuleModal(); });

function toLines(s) { return s.split("\n").map((x) => x.trim()).filter(Boolean); }
function toCsv(s) { return s.split(",").map((x) => x.trim()).filter(Boolean); }
function toPorts(s) {
  return s.split(",").map((x) => parseInt(x.trim(), 10))
    .filter((x) => Number.isInteger(x) && x > 0 && x <= 65535);
}

$("btnSaveRule").addEventListener("click", async () => {
  const name = $("ruleName").value.trim();
  if (!name) { toast("请填写规则名称"); return; }
  const body = {
    name,
    action: $("ruleAction").value,
    enabled: $("ruleEnabled").checked,
    domain_suffix: toLines($("ruleDomainSuffix").value),
    domain_keyword: toLines($("ruleDomainKeyword").value),
    domain_regex: toLines($("ruleDomainRegex").value),
    ip_cidr: toLines($("ruleIpCidr").value),
    geoip: toCsv($("ruleGeoip").value),
    geosite: toCsv($("ruleGeosite").value),
    port: toPorts($("rulePort").value),
    process: toCsv($("ruleProcess").value),
  };
  const btn = $("btnSaveRule");
  btn.disabled = true;
  try {
    if (editingRuleId != null) {
      await api(`/api/v1/routing/rules/${encodeURIComponent(editingRuleId)}`, { method: "PUT", body: JSON.stringify(body) });
      toast("已保存");
    } else {
      await api("/api/v1/routing/rules", { method: "POST", body: JSON.stringify(body) });
      toast("已新增");
    }
    closeRuleModal();
    await refreshRules();
  } catch (err) { toast("保存失败：" + err.message); }
  btn.disabled = false;
});

// ---------- GeoIP / GeoSite ----------
function fmtSize(b) {
  if (b == null) return "-";
  if (b < 1024) return b + " B";
  if (b < 1048576) return (b / 1024).toFixed(1) + " KB";
  return (b / 1048576).toFixed(1) + " MB";
}

async function refreshGeo() {
  try {
    const d = await api("/api/v1/routing/geo", { timeout: 15000 });
    state.geo = d;
    $("geoAutoUpdate").checked = !!d.auto_update;
    $("geoIntervalHours").value = d.interval_hours || 24;
    const files = Object.values(d.files || {});
    $("geoRows").innerHTML = files.map((f) => `
      <tr>
        <td class="node-name" title="${esc(f.url || "")}">${esc(f.name)}</td>
        <td class="muted">${fmtSize(f.size)}</td>
        <td class="muted">${esc(f.updated_at || "-")}</td>
        <td>${f.exists ? '<span class="compat-yes">✓ 已存在</span>' : '<span class="compat-no">✗ 缺失</span>'}</td>
      </tr>`).join("") || `<tr><td colspan="4" class="muted" style="text-align:center;padding:24px">暂无文件信息</td></tr>`;
  } catch (e) { /* 忽略 */ }
}

async function saveGeoSettings() {
  try {
    await api("/api/v1/routing/geo", {
      method: "PUT",
      timeout: 15000,
      body: JSON.stringify({
        auto_update: $("geoAutoUpdate").checked,
        interval_hours: parseInt($("geoIntervalHours").value, 10) || 24,
      }),
    });
    toast("Geo 更新设置已保存");
  } catch (err) { toast("保存失败：" + err.message); await refreshGeo(); }
}

$("geoAutoUpdate").addEventListener("change", saveGeoSettings);
$("geoIntervalHours").addEventListener("change", saveGeoSettings);

$("btnGeoUpdate").addEventListener("click", async () => {
  const btn = $("btnGeoUpdate");
  btn.disabled = true;
  try {
    await api("/api/v1/routing/geo/update", { method: "POST", timeout: 15000, body: JSON.stringify({ only_missing: false }) });
    toast("已开始后台下载，稍后自动刷新状态");
    // 后台下载中，轮询几次刷新文件状态
    let n = 0;
    const timer = setInterval(async () => {
      await refreshGeo();
      if (++n >= 6) clearInterval(timer);
    }, 3000);
  } catch (err) { toast("更新失败：" + err.message); }
  btn.disabled = false;
});

// ---------- 引擎启停 ----------
async function startEngine(nodeId) {
  try {
    const r = await api("/api/v1/engine/start", {
      method: "POST",
      body: JSON.stringify({
        node_id: nodeId || "",
        kernel: settings.kernel || undefined,
        tun: settings.tun,
        mixed_port: settings.mixedPort,
      }),
    });
    let msg = `已启动（${r.kernel}，${r.nodes} 个节点）`;
    if (r.skipped && r.skipped.length) msg += `，跳过不兼容：${r.skipped.length} 个`;
    toast(msg);
    await refreshStatus();
  } catch (err) { toast("启动失败：" + err.message); }
}

$("btnStart").addEventListener("click", () => startEngine(""));
$("btnStop").addEventListener("click", async () => {
  try { await api("/api/v1/engine/stop", { method: "POST" }); toast("已停止"); await refreshStatus(); }
  catch (err) { toast("失败：" + err.message); }
});

// ---------- 日志（SSE） ----------
function connectSSE() {
  if (state.sse) state.sse.close();
  try {
    const es = new EventSource(settings.apiBase + "/api/v1/events");
    state.sse = es;
    es.addEventListener("log", (e) => {
      try {
        const line = JSON.parse(e.data).line;
        appendLog(line);
      } catch (_) { /* 忽略 */ }
    });
    es.onerror = () => { /* 断线后浏览器自动重连 */ };
  } catch (e) { /* 忽略 */ }
}

function appendLog(line) {
  const view = $("logView");
  view.textContent += line + "\n";
  if (++state.logLines > 1500) {
    const parts = view.textContent.split("\n");
    view.textContent = parts.slice(parts.length - 1500).join("\n");
    state.logLines = 1500;
  }
  if ($("logAutoScroll").checked) view.scrollTop = view.scrollHeight;
}

$("btnClearLog").addEventListener("click", () => { $("logView").textContent = ""; state.logLines = 0; });

// ---------- 设置 ----------
function loadSettingsToForm() {
  $("setApiBase").value = settings.apiBase;
  $("setTun").checked = settings.tun;
  $("setMixedPort").value = settings.mixedPort;
  if (state.kernels.length) {
    const avail = state.kernels.find((k) => k.available && k.name === settings.kernel)
      || state.kernels.find((k) => k.available);
    if (avail) $("setKernel").value = avail.name;
  }
}

$("btnSaveSettings").addEventListener("click", () => {
  settings.apiBase = $("setApiBase").value.trim() || "http://127.0.0.1:17890";
  settings.kernel = $("setKernel").value;
  settings.tun = $("setTun").checked;
  settings.mixedPort = parseInt($("setMixedPort").value, 10) || 7890;
  connectSSE();
  refreshAll();
  toast("设置已保存");
});

$("btnViewConfig").addEventListener("click", async () => {
  try {
    const res = await fetch(settings.apiBase + "/api/v1/engine/config");
    if (!res.ok) throw new Error("暂无已生成的配置（引擎未启动过）");
    const text = await res.text();
    $("configView").textContent = JSON.stringify(JSON.parse(text), null, 2);
  } catch (err) {
    $("configView").textContent = err.message;
  }
  $("modal").classList.remove("hidden");
});
$("btnCloseModal").addEventListener("click", () => $("modal").classList.add("hidden"));
$("modal").addEventListener("click", (e) => { if (e.target.id === "modal") $("modal").classList.add("hidden"); });

// ---------- tabs ----------
document.querySelectorAll(".tabs button").forEach((b) => {
  b.addEventListener("click", () => {
    document.querySelectorAll(".tabs button").forEach((x) => x.classList.remove("active"));
    document.querySelectorAll(".tab").forEach((x) => x.classList.remove("active"));
    b.classList.add("active");
    $("tab-" + b.dataset.tab).classList.add("active");
    if (b.dataset.tab === "routing") { refreshRules(); refreshGeo(); }
  });
});

function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[c]);
}

// ---------- 启动 ----------
async function refreshAll() {
  await refreshStatus();
  await refreshKernels();
  await refreshNodes();
  await refreshSubs();
  await refreshRules();
  await refreshGeo();
  loadSettingsToForm();
}

refreshAll();
connectSSE();
setInterval(refreshStatus, 3000);
setInterval(refreshNodes, 15000);
