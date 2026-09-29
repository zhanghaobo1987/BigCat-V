package com.bigcat.v.api

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject
import java.util.concurrent.TimeUnit

/**
 * bigcatvd 本地 REST API 客户端。
 *
 * 契约见 core/internal/api/server.go（docs/API.md 待补）：
 *   GET    /api/v1/status
 *   GET    /api/v1/kernels
 *   GET    /api/v1/nodes
 *   POST   /api/v1/nodes                 {link}
 *   DELETE /api/v1/nodes/{id}
 *   POST   /api/v1/nodes/{id}/test
 *   GET    /api/v1/subscriptions
 *   POST   /api/v1/subscriptions         {name, url}
 *   POST   /api/v1/subscriptions/{id}/refresh
 *   POST   /api/v1/engine/start          {node_id, kernel, tun, mixed_port}
 *   POST   /api/v1/engine/stop
 *   GET    /api/v1/engine/config
 *   GET    /api/v1/events                (SSE: event: log, data: {"line": "..."})
 *
 * JSON 解析统一用 org.json（Android 内置，零额外依赖）。
 */
class ApiClient(baseUrl: String) {

    private val base = baseUrl.trimEnd('/')
    private val jsonMedia = "application/json; charset=utf-8".toMediaType()

    private val client = OkHttpClient.Builder()
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .writeTimeout(10, TimeUnit.SECONDS)
        .build()

    /** 服务端错误体 {"error": "..."} 统一转为异常。 */
    class ApiException(message: String) : Exception(message)

    private fun checkError(body: String, code: Int): JSONObject {
        val o = JSONObject(body)
        if (code >= 400) {
            throw ApiException(o.optString("error", "HTTP $code"))
        }
        // 部分成功响应也可能带 error 字段
        o.optString("error").takeIf { it.isNotEmpty() }?.let { throw ApiException(it) }
        return o
    }

    private suspend fun get(path: String): JSONObject = withContext(Dispatchers.IO) {
        val req = Request.Builder().url(base + path).get().build()
        client.newCall(req).execute().use { resp ->
            checkError(resp.body?.string().orEmpty(), resp.code)
        }
    }

    private suspend fun post(path: String, body: JSONObject = JSONObject()): JSONObject =
        withContext(Dispatchers.IO) {
            val req = Request.Builder()
                .url(base + path)
                .post(body.toString().toRequestBody(jsonMedia))
                .build()
            client.newCall(req).execute().use { resp ->
                checkError(resp.body?.string().orEmpty(), resp.code)
            }
        }

    private suspend fun delete(path: String): JSONObject = withContext(Dispatchers.IO) {
        val req = Request.Builder().url(base + path).delete().build()
        client.newCall(req).execute().use { resp ->
            checkError(resp.body?.string().orEmpty(), resp.code)
        }
    }

    private suspend fun getRaw(path: String): String = withContext(Dispatchers.IO) {
        val req = Request.Builder().url(base + path).get().build()
        client.newCall(req).execute().use { resp ->
            val body = resp.body?.string().orEmpty()
            if (resp.code >= 400) {
                val msg = try {
                    JSONObject(body).optString("error", "HTTP ${resp.code}")
                } catch (_: Exception) {
                    "HTTP ${resp.code}"
                }
                throw ApiException(msg)
            }
            body
        }
    }

    // ── 状态 / 内核 ──

    suspend fun getStatus(): StatusInfo = StatusInfo.fromJson(get("/api/v1/status"))

    suspend fun getKernels(): List<KernelInfo> {
        val arr = get("/api/v1/kernels").optJSONArray("kernels") ?: JSONArray()
        return List(arr.length()) { KernelInfo.fromJson(arr.getJSONObject(it)) }
    }

    // ── 节点 ──

    suspend fun getNodes(): List<Node> {
        val arr = get("/api/v1/nodes").optJSONArray("nodes") ?: JSONArray()
        return List(arr.length()) { Node.fromJson(arr.getJSONObject(it)) }
    }

    /** 添加分享链接节点，返回解析后的 Node。 */
    suspend fun addNode(link: String): Node {
        val o = post("/api/v1/nodes", JSONObject().put("link", link))
        return Node.fromJson(o)
    }

    suspend fun deleteNode(id: String) {
        delete("/api/v1/nodes/$id")
    }

    /** 测速单个节点，返回 latency_ms（-1 表示失败）。 */
    suspend fun testNode(id: String): Long {
        val o = post("/api/v1/nodes/$id/test")
        return o.optLong("latency_ms", -1)
    }

    // ── 订阅 ──

    suspend fun getSubscriptions(): List<Subscription> {
        val arr = get("/api/v1/subscriptions").optJSONArray("subscriptions") ?: JSONArray()
        return List(arr.length()) { Subscription.fromJson(arr.getJSONObject(it)) }
    }

    /** 添加订阅，返回 (订阅记录, 新增节点数)。 */
    suspend fun addSubscription(name: String, url: String): Pair<Subscription, Int> {
        val o = post(
            "/api/v1/subscriptions",
            JSONObject().put("name", name).put("url", url),
        )
        return Subscription.fromJson(o.getJSONObject("subscription")) to o.optInt("added")
    }

    /** 刷新订阅，返回新增/更新节点数。 */
    suspend fun refreshSubscription(id: String): Int {
        val o = post("/api/v1/subscriptions/$id/refresh")
        return o.optInt("added")
    }

    // ── 引擎 ──

    /**
     * 启动引擎。
     * @param nodeId 为空表示使用全部节点
     * @param kernel "sing-box" | "xray"，为空则保持当前
     * @param tun true 时生成 tun 入口（Android 上走 gVisor 用户态栈）
     */
    suspend fun startEngine(
        nodeId: String,
        kernel: String,
        tun: Boolean,
        mixedPort: Int = 7890,
    ): EngineStartResult {
        val o = post(
            "/api/v1/engine/start",
            JSONObject()
                .put("node_id", nodeId)
                .put("kernel", kernel)
                .put("tun", tun)
                .put("mixed_port", mixedPort),
        )
        return EngineStartResult.fromJson(o)
    }

    suspend fun stopEngine() {
        post("/api/v1/engine/stop")
    }

    /** 读取当前内核已生成的配置（原始 JSON 字符串）。 */
    suspend fun getEngineConfig(): String = getRaw("/api/v1/engine/config")

    // ── 日志（SSE） ──

    /**
     * 订阅 /api/v1/events 的 SSE 日志流，每行日志作为一个 String 发出。
     * 服务端事件格式：`event: log` + `data: {"line": "..."}`；另有 `event: hello` 握手。
     * 调用方 collect 即可；取消 collect 即断开连接。
     */
    fun streamLogs(): Flow<String> = callbackFlow {
        val req = Request.Builder()
            .url(base + "/api/v1/events")
            .header("Accept", "text/event-stream")
            .get()
            .build()
        val call = client.newCall(req)
        val job = launch(Dispatchers.IO) {
            try {
                call.execute().use { resp ->
                    if (resp.code >= 400) return@use
                    val src = resp.body?.source() ?: return@use
                    while (!src.exhausted()) {
                        val line = src.readUtf8Line() ?: break
                        if (!line.startsWith("data:")) continue
                        val data = line.removePrefix("data:").trim()
                        val text = try {
                            JSONObject(data).optString("line")
                        } catch (_: Exception) {
                            ""
                        }
                        if (text.isNotEmpty()) trySend(text)
                    }
                }
            } catch (_: Exception) {
                // 连接断开/核心未启动：直接结束，UI 层按需重连
            } finally {
                close()
            }
        }
        awaitClose {
            job.cancel()
            call.cancel()
        }
    }
}
