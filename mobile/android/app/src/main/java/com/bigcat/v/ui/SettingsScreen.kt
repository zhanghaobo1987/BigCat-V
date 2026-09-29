package com.bigcat.v.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.launch

/**
 * 设置页：API 地址、默认内核、核心版本/aar 状态、查看当前生成的内核配置。
 */
@Composable
fun SettingsScreen(vm: VpnViewModel, ui: VpnUiState) {
    val scope = rememberCoroutineScope()
    var apiInput by remember(ui.apiBase) { mutableStateOf(ui.apiBase) }
    var showConfig by remember { mutableStateOf(false) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
    ) {
        Text("API 地址", style = MaterialTheme.typography.titleSmall)
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            OutlinedTextField(
                value = apiInput,
                onValueChange = { apiInput = it },
                label = { Text("如 http://127.0.0.1:17890") },
                modifier = Modifier.weight(1f),
            )
        }
        Button(
            onClick = { vm.setApiBase(apiInput) },
            enabled = apiInput.isNotBlank(),
        ) { Text("保存并重连") }

        HorizontalDivider(modifier = Modifier.padding(vertical = 16.dp))

        Text("默认内核", style = MaterialTheme.typography.titleSmall)
        KernelOption("自动", "", ui) { vm.setKernel(it) }
        ui.kernels.forEach { k ->
            KernelOption(
                label = "${k.name}" + if (k.available) "" else "（不可用）",
                value = k.name,
                ui = ui,
                enabled = k.available,
                onSelect = { vm.setKernel(it) },
            )
        }
        // 内核列表为空（核心未启动）时仍允许手动选择
        if (ui.kernels.isEmpty()) {
            KernelOption("sing-box", "sing-box", ui) { vm.setKernel(it) }
            KernelOption("xray", "xray", ui) { vm.setKernel(it) }
        }

        HorizontalDivider(modifier = Modifier.padding(vertical = 16.dp))

        Text("核心", style = MaterialTheme.typography.titleSmall)
        Text(
            text = "版本：${ui.coreVersion}\n" +
                "gomobile aar：${if (ui.coreAvailable) "已接入" else "未接入（见 mobile/android/README.md）"}",
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(vertical = 8.dp),
        )
        OutlinedButton(onClick = { showConfig = true }) {
            Text("查看当前内核配置")
        }
    }

    if (showConfig) {
        var config by remember { mutableStateOf("加载中…") }
        LaunchedEffect(Unit) {
            config = try {
                vm.fetchEngineConfig()
            } catch (e: Exception) {
                "读取失败：${e.message}"
            }
        }
        AlertDialog(
            onDismissRequest = { showConfig = false },
            title = { Text("内核配置") },
            text = {
                Text(
                    text = config,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 11.sp,
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(max = 400.dp)
                        .verticalScroll(rememberScrollState()),
                )
            },
            confirmButton = {
                TextButton(onClick = { showConfig = false }) { Text("关闭") }
            },
        )
    }
}

@Composable
private fun KernelOption(
    label: String,
    value: String,
    ui: VpnUiState,
    enabled: Boolean = true,
    onSelect: (String) -> Unit = {},
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(
            selected = ui.kernel == value,
            onClick = { if (enabled) onSelect(value) },
            enabled = enabled,
        )
        Text(
            text = label,
            color = if (enabled) MaterialTheme.colorScheme.onSurface
            else MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}
