package com.bigcat.v.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Speed
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/**
 * 节点页：列表（名称/协议/延迟/选择）+ 连接开关 + 测速 + 添加/删除。
 * 连接动作经 onConnectRequest 交给 MainActivity（需先走 VpnService.prepare 授权）。
 */
@Composable
fun NodesScreen(
    vm: VpnViewModel,
    ui: VpnUiState,
    onConnectRequest: (nodeId: String, kernel: String) -> Unit,
) {
    var showAdd by remember { mutableStateOf(false) }
    var link by remember { mutableStateOf("") }

    Scaffold(
        floatingActionButton = {
            FloatingActionButton(onClick = { showAdd = true }) {
                Icon(Icons.Filled.Add, contentDescription = "添加节点")
            }
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(12.dp),
        ) {
            // 连接控制行
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Button(
                    onClick = {
                        if (ui.connected) vm.disconnect()
                        else onConnectRequest(ui.selectedNodeId, ui.kernel)
                    },
                    enabled = ui.connected || ui.selectedNodeId.isNotEmpty(),
                ) {
                    Icon(
                        if (ui.connected) Icons.Filled.Stop else Icons.Filled.PlayArrow,
                        contentDescription = null,
                    )
                    Text(if (ui.connected) "断开" else "连接")
                }
                OutlinedButton(onClick = { vm.testAllNodes() }) {
                    Icon(Icons.Filled.Speed, contentDescription = null)
                    Text("测速全部")
                }
                IconButton(onClick = { vm.refreshNodes() }) {
                    Icon(Icons.Filled.Refresh, contentDescription = "刷新")
                }
                if (ui.loading) CircularProgressIndicator()
            }

            if (ui.connected) {
                val activeName = ui.nodes.find { it.id == ui.activeNodeId }?.displayName
                Text(
                    text = if (activeName != null) "已连接 · $activeName" else "已连接",
                    color = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.padding(vertical = 4.dp),
                )
            }
            if (ui.lastEngineError.isNotEmpty() && !ui.connected) {
                Text(
                    text = "引擎错误：${ui.lastEngineError}",
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(vertical = 4.dp),
                )
            }

            HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))

            LazyColumn(modifier = Modifier.fillMaxSize()) {
                items(ui.nodes, key = { it.id }) { node ->
                    ListItem(
                        headlineContent = { Text(node.displayName) },
                        supportingContent = {
                            Column {
                                Text("${node.protocol} · ${node.server}:${node.port}")
                                if (!node.xrayCompatible && ui.kernel == "xray") {
                                    Text(
                                        text = "不兼容 xray：${node.xrayNote}",
                                        color = MaterialTheme.colorScheme.error,
                                    )
                                }
                            }
                        },
                        leadingContent = {
                            RadioButton(
                                selected = node.id == ui.selectedNodeId,
                                onClick = { vm.selectNode(node.id) },
                            )
                        },
                        trailingContent = {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text(
                                    text = node.latencyText,
                                    color = if (node.latencyMs < 0)
                                        MaterialTheme.colorScheme.onSurfaceVariant
                                    else
                                        MaterialTheme.colorScheme.primary,
                                )
                                IconButton(onClick = { vm.testNode(node.id) }) {
                                    Icon(Icons.Filled.Speed, contentDescription = "测速")
                                }
                                IconButton(onClick = { vm.deleteNode(node.id) }) {
                                    Icon(Icons.Filled.Delete, contentDescription = "删除")
                                }
                            }
                        },
                        modifier = Modifier.clickable { vm.selectNode(node.id) },
                    )
                    HorizontalDivider()
                }
            }
        }
    }

    if (showAdd) {
        AlertDialog(
            onDismissRequest = { showAdd = false },
            title = { Text("添加节点") },
            text = {
                OutlinedTextField(
                    value = link,
                    onValueChange = { link = it },
                    label = { Text("分享链接（ss/vmess/vless/trojan/hy2/tuic…）") },
                    modifier = Modifier.fillMaxWidth(),
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    vm.addNode(link) { showAdd = false; link = "" }
                }) { Text("添加") }
            },
            dismissButton = {
                TextButton(onClick = { showAdd = false }) { Text("取消") }
            },
        )
    }
}
