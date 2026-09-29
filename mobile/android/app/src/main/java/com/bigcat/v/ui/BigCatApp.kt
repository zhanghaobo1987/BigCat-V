package com.bigcat.v.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Article
import androidx.compose.material.icons.filled.List
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Subscriptions
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.lifecycle.viewmodel.compose.viewModel

private enum class Tab(val title: String, val icon: ImageVector) {
    NODES("节点", Icons.Filled.List),
    SUBS("订阅", Icons.Filled.Subscriptions),
    LOGS("日志", Icons.Filled.Article),
    SETTINGS("设置", Icons.Filled.Settings),
}

/**
 * 应用导航壳：顶部栏 + 底部 4 Tab。
 * @param onConnectRequest 发起连接（MainActivity 负责 VPN 授权流程）
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BigCatApp(onConnectRequest: (nodeId: String, kernel: String) -> Unit) {
    val vm: VpnViewModel = viewModel()
    val ui by vm.ui.collectAsState()
    var tab by remember { mutableStateOf(Tab.NODES) }
    val snackbar = remember { SnackbarHostState() }

    // ViewModel 的 error 统一转成 Snackbar
    LaunchedEffect(ui.error) {
        ui.error?.let {
            snackbar.showSnackbar(it)
            vm.clearError()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text("BigCat V" + if (ui.connected) " · 运行中" else "")
                },
            )
        },
        bottomBar = {
            NavigationBar {
                Tab.entries.forEach { t ->
                    NavigationBarItem(
                        selected = tab == t,
                        onClick = { tab = t },
                        icon = { Icon(t.icon, contentDescription = t.title) },
                        label = { Text(t.title) },
                    )
                }
            }
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Box(modifier = Modifier.padding(padding)) {
            when (tab) {
                Tab.NODES -> NodesScreen(vm, ui, onConnectRequest)
                Tab.SUBS -> SubscriptionsScreen(vm, ui)
                Tab.LOGS -> LogsScreen(ui.apiBase)
                Tab.SETTINGS -> SettingsScreen(vm, ui)
            }
        }
    }
}
