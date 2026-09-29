# BigCat V 控制 API（v0.4）

本地 REST + SSE，默认监听 `127.0.0.1:17890`。这是 bigcatvd 对外唯一的界面，
各端 UI（Tauri 桌面 / iOS / Android）只调这里。错误统一为 `{"error": "..."}`。

## 基础

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/status | 版本、运行中内核、当前节点 |
| GET | /api/v1/kernels | 内核可用性：`{"sing-box": {"available": true, "binary": "..."}, "xray": {...}}` |
| GET | /api/v1/events | SSE 事件流（日志/状态） |

## 节点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/nodes | 节点列表（含 `latency_ms`、`xray_compatible`） |
| POST | /api/v1/nodes | 批量导入分享链接 `{"links": [...]}` |
| DELETE | /api/v1/nodes/{id} | 删除节点 |
| POST | /api/v1/nodes/{id}/test | 测速，返回 `{"latency_ms": n}` |

## 订阅

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/subscriptions | 订阅列表 |
| POST | /api/v1/subscriptions | `{"name": "", "url": ""}` 添加并抓取 |
| POST | /api/v1/subscriptions/{id}/refresh | 刷新订阅 |

## 引擎

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/engine/start | `{"node_id": "", "kernel": "sing-box"\|"xray", "tun": false, "mixed_port": 7890}` |
| POST | /api/v1/engine/stop | 停止引擎 |
| GET | /api/v1/engine/config | 当前生成的内核配置 JSON |

## 分流规则（v0.4）

规则按列表顺序匹配，首条命中生效；同一规则内多个条件为"或"关系。
动作：`proxy`（走代理）/ `direct`（直连）/ `block`（拦截）。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/routing/rules | `{"default_action": "proxy", "rules": [...]}` |
| POST | /api/v1/routing/rules | 新增（ID 自动生成），body 为规则对象（不含 id） |
| PUT | /api/v1/routing/rules/{id} | 全量更新规则 |
| DELETE | /api/v1/routing/rules/{id} | 删除 |
| POST | /api/v1/routing/rules/{id}/toggle | `{"enabled": bool}` 启用/禁用 |
| POST | /api/v1/routing/rules/reorder | `{"ids": [...]}` 按 ID 顺序重排 |
| PUT | /api/v1/routing/default-action | `{"default_action": "proxy"}` 未命中时的动作 |

规则对象字段：`id`、`name`、`enabled`、`action`、`domain_suffix[]`、
`domain_keyword[]`、`domain_regex[]`、`ip_cidr[]`、`geoip[]`（如 `cn`）、
`geosite[]`（如 `cn` / `category-ads-all`）、`port[]`、`process[]`（仅 sing-box 生效）。

注意：
- xray 不支持 `process` 条件（会被丢弃）；
- geo 条件需要本地 geo 库存在（见下），缺失时该条件被丢弃、规则无剩余条件则整条跳过。

## GeoIP / GeoSite（v0.4）

daemon 启动时自动补齐缺失的 geo 文件，并按设置定时全量更新。
sing-box 使用本地 `.srs` rule_set，xray 使用 `geoip.dat` / `geosite.dat`
（`XRAY_LOCATION_ASSET` 已指向 geo 目录）。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/routing/geo | `{"auto_update": true, "interval_hours": 24, "files": {"geoip-cn.srs": {"name","url","size","updated_at","exists"}}}` |
| POST | /api/v1/routing/geo/update | `{"only_missing": false}` 触发后台下载，返回 `202 {"accepted": true}` |
| PUT | /api/v1/routing/geo | `{"auto_update": bool, "interval_hours": n}` |
