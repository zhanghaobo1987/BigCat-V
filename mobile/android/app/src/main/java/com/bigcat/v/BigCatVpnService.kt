package com.bigcat.v

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.ParcelFileDescriptor
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import com.bigcat.v.api.ApiClient
import com.bigcat.v.core.CoreBridge
import com.bigcat.v.ui.Prefs
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * BigCat V 的 Android VPN 服务。
 *
 * 连接流程（ACTION_CONNECT）：
 *  1. VpnService.Builder 建立系统 TUN：
 *     addAddress("198.18.0.1", 24) + addRoute("0.0.0.0", 0)，全部流量走隧道；
 *     自身应用加入 disallow 列表，避免 API 回环。
 *  2. CoreBridge.start("127.0.0.1:17890", filesDir) 拉起 Go 核心（gomobile）。
 *  3. ApiClient POST /api/v1/engine/start {tun: true} 启动引擎。
 *
 * ── 关于 tun 模式与 gVisor 用户态栈 ──────────────────────────────
 * tun=true 时生成的 sing-box 配置包含 tun 入口（tag "tun-in"），其
 * stack 默认为 "gvisor"。gVisor 是纯用户态 TCP/IP 协议栈：
 * sing-box 在用户态完成 TCP 重组、UDP 会话管理与路由，
 * 不依赖系统 TUN 设备创建权限、无需 root 即可处理三层包，
 * 这正是移动端/无特权环境跑 TUN 的标准做法。
 *
 * ── 关于 Android TUN 文件描述符 ─────────────────────────────────
 * 本服务建立的 TUN 是 Android 系统 VPN 接口，三层包由此进出。
 * 生产级数据面需要把 tunFd.detachFd() 得到的 fd 交给 sing-box 的
 * tun 入口（sing-box 的 Android 特有选项 "file_descriptor"），
 * 待 core/mobile 暴露 SetTunFd(fd: Int) 后在此处 wiring，
 * 并将生成器 Options 扩展为 TunFd 模式。v0.3 骨架阶段先打通
 * “建 TUN → 拉核心 → 起引擎”的控制面链路。
 * ───────────────────────────────────────────────────────────────
 *
 * 断开流程（ACTION_DISCONNECT / onRevoke）：调 engine/stop → CoreBridge.stop()
 * → 关闭 TUN fd → 停前台服务。
 */
class BigCatVpnService : VpnService() {

    companion object {
        const val ACTION_CONNECT = "com.bigcat.v.action.CONNECT"
        const val ACTION_DISCONNECT = "com.bigcat.v.action.DISCONNECT"
        const val EXTRA_NODE_ID = "extra_node_id"
        const val EXTRA_KERNEL = "extra_kernel"

        private const val NOTIF_ID = 1
        private const val CHANNEL_ID = "bigcatv_vpn"

        /** UI 层发起连接的入口（已处理 VPN 授权的前提下调用）。 */
        fun connect(context: Context, nodeId: String, kernel: String) {
            val intent = Intent(context, BigCatVpnService::class.java).apply {
                action = ACTION_CONNECT
                putExtra(EXTRA_NODE_ID, nodeId)
                putExtra(EXTRA_KERNEL, kernel)
            }
            ContextCompat.startForegroundService(context, intent)
        }

        fun disconnect(context: Context) {
            val intent = Intent(context, BigCatVpnService::class.java).apply {
                action = ACTION_DISCONNECT
            }
            context.startService(intent)
        }
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var tunFd: ParcelFileDescriptor? = null

    override fun onCreate() {
        super.onCreate()
        createChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_CONNECT -> {
                val nodeId = intent.getStringExtra(EXTRA_NODE_ID).orEmpty()
                val kernel = intent.getStringExtra(EXTRA_KERNEL).orEmpty()
                startForeground(NOTIF_ID, buildNotification("正在连接…"))
                scope.launch { doConnect(nodeId, kernel) }
            }
            ACTION_DISCONNECT -> scope.launch { doDisconnect(null) }
        }
        return START_STICKY
    }

    /** 用户在系统设置里撤销 VPN 授权时被回调。 */
    override fun onRevoke() {
        scope.launch { doDisconnect("VPN 授权被撤销") }
    }

    override fun onDestroy() {
        scope.launch { doDisconnect(null) }
        scope.cancel()
        super.onDestroy()
    }

    private suspend fun doConnect(nodeId: String, kernel: String) {
        try {
            // 1) 建立系统 TUN
            val builder = Builder()
                .setSession("BigCat V")
                .setMtu(1500)
                .addAddress("198.18.0.1", 24)
                .addRoute("0.0.0.0", 0)
                .addDnsServer("8.8.8.8")
                .addDnsServer("1.1.1.1")
            // 自身走直连，避免本地 API 回环进隧道
            runCatching { builder.addDisallowedApplication(packageName) }
            tunFd = builder.establish()
                ?: throw IllegalStateException("TUN 建立失败（用户拒绝了 VPN 授权？）")

            // 2) 拉起 Go 核心（gomobile aar）
            val apiBase = Prefs.getApiBase(this)
            val listenAddr = apiBase.removePrefix("http://").removePrefix("https://")
            CoreBridge.start(listenAddr, filesDir.absolutePath)

            // 3) 通过本地 API 启动引擎（tun=true → sing-box gVisor 用户态栈）
            val result = ApiClient(apiBase).startEngine(
                nodeId = nodeId,
                kernel = kernel.ifEmpty { Prefs.getKernel(this) },
                tun = true,
                mixedPort = 7890,
            )
            val skipNote = if (result.skipped.isNotEmpty()) "（跳过 ${result.skipped.size} 个不兼容节点）" else ""
            updateNotification("已连接 · ${result.kernel}$skipNote")
        } catch (e: Exception) {
            updateNotification("连接失败：${e.message}")
            doDisconnect(null)
        }
    }

    private suspend fun doDisconnect(reason: String?) {
        runCatching {
            // 先停引擎，再停核心，最后关 TUN：顺序与启动相反
            ApiClient(Prefs.getApiBase(this)).stopEngine()
        }
        runCatching { CoreBridge.stop() }
        runCatching { tunFd?.close() }
        tunFd = null
        if (reason != null) updateNotification(reason)
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    // ── 前台服务通知 ──

    private fun createChannel() {
        val channel = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.vpn_channel_name),
            NotificationManager.IMPORTANCE_LOW,
        ).apply { description = getString(R.string.vpn_channel_desc) }
        getSystemService(NotificationManager::class.java)?.createNotificationChannel(channel)
    }

    private fun buildNotification(text: String): Notification {
        val pending = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_vpn)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setContentIntent(pending)
            .setOngoing(true)
            .build()
    }

    private fun updateNotification(text: String) {
        getSystemService(NotificationManager::class.java)
            ?.notify(NOTIF_ID, buildNotification(text))
    }
}
