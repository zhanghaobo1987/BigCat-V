package com.bigcat.v.api

import org.json.JSONObject

// ── 数据模型：与 core/internal/api 返回的 JSON 字段一一对应 ────────────────

/** 统一节点（含 xray 兼容性标记，来自 GET /api/v1/nodes 的 nodeView）。 */
data class Node(
    val id: String,
    val name: String,
    val protocol: String,
    val server: String,
    val port: Int,
    val group: String,
    /** -1 = 未测/失败，>=0 为毫秒数 */
    val latencyMs: Long,
    val xrayCompatible: Boolean,
    val xrayNote: String,
) {
    val displayName: String get() = if (group.isNotEmpty()) "[$group] $name" else name
    val latencyText: String
        get() = when {
            latencyMs < 0 -> "未测"
            else -> "${latencyMs}ms"
        }

    companion object {
        fun fromJson(o: JSONObject) = Node(
            id = o.optString("id"),
            name = o.optString("name"),
            protocol = o.optString("protocol"),
            server = o.optString("server"),
            port = o.optInt("port"),
            group = o.optString("group"),
            latencyMs = o.optLong("latency_ms", -1),
            xrayCompatible = o.optBoolean("xray_compatible", true),
            xrayNote = o.optString("xray_note"),
        )
    }
}

/** 内核可用性（GET /api/v1/kernels）。 */
data class KernelInfo(
    val name: String,
    val available: Boolean,
    val binary: String,
    val version: String,
) {
    companion object {
        fun fromJson(o: JSONObject) = KernelInfo(
            name = o.optString("name"),
            available = o.optBoolean("available"),
            binary = o.optString("binary"),
            version = o.optString("version"),
        )
    }
}

/** 引擎状态快照（GET /api/v1/status 的 engine 字段）。 */
data class EngineSnapshot(
    /** "stopped" | "running" | "error" */
    val status: String,
    val binary: String,
    val restarts: Int,
    val lastError: String,
) {
    val isRunning: Boolean get() = status == "running"

    companion object {
        fun fromJson(o: JSONObject) = EngineSnapshot(
            status = o.optString("status"),
            binary = o.optString("binary"),
            restarts = o.optInt("restarts"),
            lastError = o.optString("last_error"),
        )
    }
}

/** GET /api/v1/status。 */
data class StatusInfo(
    val version: String,
    val kernel: String,
    val engine: EngineSnapshot?,
    val activeNode: String,
    val nodeCount: Int,
) {
    companion object {
        fun fromJson(o: JSONObject) = StatusInfo(
            version = o.optString("version"),
            kernel = o.optString("kernel"),
            engine = o.optJSONObject("engine")?.let(EngineSnapshot::fromJson),
            activeNode = o.optString("active_node"),
            nodeCount = o.optInt("node_count"),
        )
    }
}

/** 订阅记录（GET /api/v1/subscriptions）。 */
data class Subscription(
    val id: String,
    val name: String,
    val url: String,
    val count: Int,
) {
    companion object {
        fun fromJson(o: JSONObject) = Subscription(
            id = o.optString("id"),
            name = o.optString("name"),
            url = o.optString("url"),
            count = o.optInt("count"),
        )
    }
}

/** POST /api/v1/engine/start 的成功响应。 */
data class EngineStartResult(
    val started: Boolean,
    val kernel: String,
    val config: String,
    val nodes: Int,
    val skipped: List<String>,
) {
    companion object {
        fun fromJson(o: JSONObject): EngineStartResult {
            val skipped = mutableListOf<String>()
            o.optJSONArray("skipped")?.let { arr ->
                for (i in 0 until arr.length()) skipped += arr.optString(i)
            }
            return EngineStartResult(
                started = o.optBoolean("started"),
                kernel = o.optString("kernel"),
                config = o.optString("config"),
                nodes = o.optInt("nodes"),
                skipped = skipped,
            )
        }
    }
}
