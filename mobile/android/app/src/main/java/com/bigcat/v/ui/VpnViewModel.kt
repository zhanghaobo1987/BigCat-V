package com.bigcat.v.ui

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.bigcat.v.BigCatVpnService
import com.bigcat.v.api.ApiClient
import com.bigcat.v.api.KernelInfo
import com.bigcat.v.api.Node
import com.bigcat.v.api.Subscription
import com.bigcat.v.core.CoreBridge
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/** 全应用 UI 状态。 */
data class VpnUiState(
    val apiBase: String = Prefs.DEFAULT_API_BASE,
    val kernel: String = "",
    val kernels: List<KernelInfo> = emptyList(),
    val nodes: List<Node> = emptyList(),
    val selectedNodeId: String = "",
    val subscriptions: List<Subscription> = emptyList(),
    val engineStatus: String = "stopped",
    val activeNodeId: String = "",
    val lastEngineError: String = "",
    val coreVersion: String = "unknown",
    val coreAvailable: Boolean = false,
    val loading: Boolean = false,
    val error: String? = null,
) {
    val connected: Boolean get() = engineStatus == "running"
}

/**
 * 主 ViewModel：封装全部 API 调用。
 * 注意 VPN 授权必须走 Activity（VpnService.prepare），因此 connect 由
 * MainActivity 传入的 onConnectRequest 回调发起，这里只负责断开与状态轮询。
 */
class VpnViewModel(app: Application) : AndroidViewModel(app) {

    private val _ui = MutableStateFlow(VpnUiState())
    val ui: StateFlow<VpnUiState> = _ui.asStateFlow()

    private var pollJob: Job? = null

    private fun api() = ApiClient(_ui.value.apiBase)

    init {
        _ui.update {
            it.copy(
                apiBase = Prefs.getApiBase(app),
                kernel = Prefs.getKernel(app),
                coreVersion = CoreBridge.version(),
                coreAvailable = CoreBridge.isAvailable(),
            )
        }
        refreshAll()
        startStatusPolling()
    }

    // ── 轮询引擎状态 ──

    private fun startStatusPolling() {
        pollJob?.cancel()
        pollJob = viewModelScope.launch {
            while (isActive) {
                try {
                    val st = api().getStatus()
                    _ui.update {
                        it.copy(
                            engineStatus = st.engine?.status ?: "stopped",
                            activeNodeId = st.activeNode,
                            lastEngineError = st.engine?.lastError.orEmpty(),
                        )
                    }
                } catch (_: Exception) {
                    // 核心未启动时 API 不通，属正常
                    _ui.update { it.copy(engineStatus = "stopped") }
                }
                delay(3000)
            }
        }
    }

    // ── 节点 ──

    fun refreshNodes() = viewModelScope.launch {
        _ui.update { it.copy(loading = true) }
        try {
            val nodes = api().getNodes().sortedWith(
                compareBy({ it.latencyMs < 0 }, { it.latencyMs }),
            )
            _ui.update {
                it.copy(
                    nodes = nodes,
                    loading = false,
                    // 默认选中第一个节点
                    selectedNodeId = it.selectedNodeId
                        .takeIf { id -> nodes.any { n -> n.id == id } }
                        ?: nodes.firstOrNull()?.id.orEmpty(),
                )
            }
        } catch (e: Exception) {
            _ui.update { it.copy(loading = false, error = "刷新节点失败：${e.message}") }
        }
    }

    fun selectNode(id: String) {
        _ui.update { it.copy(selectedNodeId = id) }
    }

    fun addNode(link: String, onDone: () -> Unit = {}) = viewModelScope.launch {
        try {
            api().addNode(link.trim())
            refreshNodes()
            onDone()
        } catch (e: Exception) {
            _ui.update { it.copy(error = "添加节点失败：${e.message}") }
        }
    }

    fun deleteNode(id: String) = viewModelScope.launch {
        try {
            api().deleteNode(id)
            refreshNodes()
        } catch (e: Exception) {
            _ui.update { it.copy(error = "删除失败：${e.message}") }
        }
    }

    fun testNode(id: String) = viewModelScope.launch {
        try {
            val ms = api().testNode(id)
            _ui.update {
                it.copy(nodes = it.nodes.map { n -> if (n.id == id) n.copy(latencyMs = ms) else n })
            }
        } catch (e: Exception) {
            _ui.update { it.copy(error = "测速失败：${e.message}") }
        }
    }

    /** 逐个测速全部节点（服务端为 TCP 握手延迟，并发由服务端控制，这里串行即可）。 */
    fun testAllNodes() = viewModelScope.launch {
        _ui.update { it.copy(loading = true) }
        try {
            val updated = _ui.value.nodes.map { n ->
                n.copy(latencyMs = api().testNode(n.id))
            }.sortedWith(compareBy({ it.latencyMs < 0 }, { it.latencyMs }))
            _ui.update { it.copy(nodes = updated, loading = false) }
        } catch (e: Exception) {
            _ui.update { it.copy(loading = false, error = "测速失败：${e.message}") }
        }
    }

    // ── 订阅 ──

    fun refreshSubs() = viewModelScope.launch {
        try {
            _ui.update { it.copy(subscriptions = api().getSubscriptions()) }
        } catch (e: Exception) {
            _ui.update { it.copy(error = "刷新订阅失败：${e.message}") }
        }
    }

    fun addSubscription(name: String, url: String, onDone: () -> Unit = {}) =
        viewModelScope.launch {
            try {
                api().addSubscription(name.trim(), url.trim())
                refreshSubs()
                refreshNodes()
                _ui.update { it.copy(error = null) }
                onDone()
            } catch (e: Exception) {
                _ui.update { it.copy(error = "添加订阅失败：${e.message}") }
            }
        }

    fun refreshSubscription(id: String) = viewModelScope.launch {
        try {
            api().refreshSubscription(id)
            refreshSubs()
            refreshNodes()
        } catch (e: Exception) {
            _ui.update { it.copy(error = "刷新订阅失败：${e.message}") }
        }
    }

    // ── 设置 ──

    fun refreshKernels() = viewModelScope.launch {
        try {
            _ui.update { it.copy(kernels = api().getKernels()) }
        } catch (_: Exception) {
            // 核心未启动时忽略
        }
    }

    fun setKernel(kernel: String) {
        Prefs.setKernel(getApplication(), kernel)
        _ui.update { it.copy(kernel = kernel) }
    }

    fun setApiBase(url: String) {
        val clean = url.trim().trimEnd('/')
        Prefs.setApiBase(getApplication(), clean)
        _ui.update { it.copy(apiBase = clean) }
        refreshAll()
        startStatusPolling()
    }

    suspend fun fetchEngineConfig(): String = api().getEngineConfig()

    // ── 连接控制 ──

    fun disconnect() {
        BigCatVpnService.disconnect(getApplication())
    }

    fun refreshAll() {
        refreshNodes()
        refreshSubs()
        refreshKernels()
    }

    fun clearError() {
        _ui.update { it.copy(error = null) }
    }
}
