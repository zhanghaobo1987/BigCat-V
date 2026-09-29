package com.bigcat.v

import android.Manifest
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import com.bigcat.v.ui.BigCatApp

/**
 * 唯一 Activity：承载 Compose UI，并负责 VPN 授权流程。
 *
 * 连接必须先经 VpnService.prepare() 拿到用户授权（返回 null 表示已授权，
 * 否则拿到 Intent 拉起系统授权页）。授权结果通过
 * ActivityResultLauncher 回调，再执行真正的 BigCatVpnService.connect()。
 */
class MainActivity : ComponentActivity() {

    /** 授权通过后待执行的连接动作。 */
    private var pendingConnect: (() -> Unit)? = null

    private val vpnAuthLauncher =
        registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
            if (result.resultCode == RESULT_OK) {
                pendingConnect?.invoke()
            }
            pendingConnect = null
        }

    private val notifPermLauncher =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) {
            // 通知权限被拒不影响核心功能，仅收不到前台服务通知的横幅
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Android 13+ 前台服务通知需要运行时申请
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            notifPermLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
        }

        setContent {
            MaterialTheme(colorScheme = darkColorScheme()) {
                BigCatApp(onConnectRequest = { nodeId, kernel ->
                    val prepareIntent = VpnService.prepare(this)
                    val doConnect = { BigCatVpnService.connect(this, nodeId, kernel) }
                    if (prepareIntent != null) {
                        // 尚未授权：先拉起系统授权页，回调里再连接
                        pendingConnect = doConnect
                        vpnAuthLauncher.launch(prepareIntent)
                    } else {
                        doConnect()
                    }
                })
            }
        }
    }
}
