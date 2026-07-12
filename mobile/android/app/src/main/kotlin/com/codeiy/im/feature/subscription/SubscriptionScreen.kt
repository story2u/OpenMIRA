package com.codeiy.im.feature.subscription

import android.app.Activity
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.codeiy.im.core.auth.SessionStore
import com.codeiy.im.model.BillingInterval
import com.codeiy.im.model.PlanCode

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SubscriptionScreen(session: SessionStore, onBack: () -> Unit) {
    val user = session.currentUser ?: return
    val model = remember(user.id) { SubscriptionViewModel(user.id, SessionSubscriptionBackend(session), session.billing) }
    val state by model.state.collectAsState()
    val activity = LocalContext.current as Activity
    LaunchedEffect(model) { model.load() }

    Scaffold(topBar = { TopAppBar(
        title = { Text("套餐与用量") },
        navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
        actions = { TextButton(onClick = { model.restore() }, enabled = session.billing.isConfigured && !state.restoring) { Text("恢复购买") } },
    ) }) { padding ->
        LazyColumn(modifier = Modifier.padding(padding).padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            state.usage?.let { usage ->
                item {
                    Card(Modifier.fillMaxWidth()) { Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text("当前套餐 ${usage.planCode.label}", style = MaterialTheme.typography.titleMedium)
                        usage.effectiveStore?.let { Text("购买渠道：${it.label}") }
                        Text("AI 分析 ${usage.aiAnalysesConsumed + usage.aiAnalysesReserved} / ${usage.entitlements.piAgentAnalysisMonthlyLimit}")
                        Text("群监控 ${usage.combinedGroupsUsed} / ${usage.entitlements.combinedGroupLimit}")
                        if (usage.cancelAtPeriodEnd) Text("续费已取消，当前周期结束前权益保持有效", color = MaterialTheme.colorScheme.tertiary)
                        state.management?.let { management ->
                            Button(onClick = { management.managementUrl?.let { activity.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(it))) } }, enabled = management.canOpenInCurrentClient && management.managementUrl != null) { Text("管理订阅") }
                            if (!management.canOpenInCurrentClient) Text(management.instruction, style = MaterialTheme.typography.bodySmall)
                        }
                    } }
                }
                if (usage.multipleActiveSubscriptions) item { Text("检测到多个渠道的有效订阅，你可能正在重复付费。请前往原购买渠道管理。", color = MaterialTheme.colorScheme.tertiary) }
                if (usage.billingIssue) item { Text("当前订阅存在付款问题，请前往原购买渠道处理。", color = MaterialTheme.colorScheme.error) }
                item {
                    SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                        listOf(BillingInterval.MONTHLY to "月付", BillingInterval.ANNUAL to "年付").forEachIndexed { index, (interval, label) ->
                            SegmentedButton(selected = state.interval == interval, onClick = { model.setInterval(interval) }, shape = SegmentedButtonDefaults.itemShape(index, 2)) { Text(label) }
                        }
                    }
                }
                if (!session.billing.isConfigured) item { Text("支付尚未配置", color = MaterialTheme.colorScheme.onSurfaceVariant) }
                items(state.catalog, key = { it.planCode.name }) { plan ->
                    val option = model.packageFor(plan.planCode)
                    Card(Modifier.fillMaxWidth()) { Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) { Text(plan.displayName, style = MaterialTheme.typography.titleMedium); Text(if (plan.planCode == PlanCode.FREE) "免费" else option?.localizedPrice ?: "价格暂不可用") }
                        Text("每月 ${plan.entitlements.piAgentAnalysisMonthlyLimit} 次 AI 分析 · ${plan.entitlements.combinedGroupLimit} 个群")
                        if (plan.planCode != PlanCode.FREE && plan.planCode != usage.planCode) Button(onClick = { option?.let { model.purchase(activity, it) } }, enabled = usage.planCode == PlanCode.FREE && option != null && state.busyPackageId == null) { Text(if (state.busyPackageId == option?.id) "处理中…" else if (usage.planCode != PlanCode.FREE) "请先管理现有订阅" else "购买") }
                    } }
                }
                if (usage.planCode != PlanCode.FREE) item { Text("已有有效订阅。请先在原购买渠道管理，避免重复付费。") }
            }
            state.message?.let { item { Text(it) } }
            state.error?.let { item { Text(it, color = MaterialTheme.colorScheme.error) } }
        }
    }
}
