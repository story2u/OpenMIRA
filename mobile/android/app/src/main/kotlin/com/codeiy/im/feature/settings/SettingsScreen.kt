package com.codeiy.im.feature.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Groups
import androidx.compose.material.icons.filled.AccountBalanceWallet
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.material.icons.filled.Send
import androidx.compose.material.icons.filled.Label
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Work
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.codeiy.im.core.auth.SessionStore
import com.codeiy.im.ui.theme.AppColors

sealed interface SettingsRoute {
    data object Security : SettingsRoute
    data object Subscription : SettingsRoute
    data object Telegram : SettingsRoute
    data object Detection : SettingsRoute
    data object WorkSchedule : SettingsRoute
    data object Notifications : SettingsRoute
    data object JobSearch : SettingsRoute
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(session: SessionStore, onNavigate: (SettingsRoute) -> Unit) {
    val model: SettingsViewModel = viewModel { SettingsViewModel(session.api.service) }
    val state by model.state.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) { model.load() }

    Scaffold(topBar = { TopAppBar(title = { Text("设置中心") }) }) { padding ->
        LazyColumn(Modifier.padding(padding).fillMaxSize(), contentPadding = androidx.compose.foundation.layout.PaddingValues(vertical = 8.dp)) {
            item { userHeader(session) }

            if (state.loadError != null && state.bundle == null) {
                item {
                    Column(Modifier.fillMaxWidth().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                        Text(state.loadError ?: "", color = AppColors.destructive)
                        TextButton(onClick = { model.load() }) { Text("重试") }
                    }
                }
            }

            item { sectionHeader("平台绑定") }
            item {
                settingsRow(Icons.Filled.Lock, AppColors.success, "账户安全") { onNavigate(SettingsRoute.Security) }
            }
            item {
                settingsRow(Icons.Filled.AccountBalanceWallet, AppColors.ai, "套餐与用量") { onNavigate(SettingsRoute.Subscription) }
            }
            item {
                settingsRow(Icons.Filled.Send, AppColors.telegram, "Telegram 连接") { onNavigate(SettingsRoute.Telegram) }
            }
            item {
                // 企业微信无用户级绑定：诚实标注，不做假连接入口。
                Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Filled.Groups, contentDescription = null, tint = AppColors.wecom)
                    Spacer(Modifier.size(12.dp))
                    Text("企业微信", Modifier.weight(1f))
                    Text("由管理员配置", style = MaterialTheme.typography.labelSmall, color = AppColors.muted)
                }
            }

            item { sectionHeader("识别与自动化") }
            if (state.bundle != null) {
                item { settingsRow(Icons.Filled.Work, AppColors.success, "求职档案") { onNavigate(SettingsRoute.JobSearch) } }
                item { settingsRow(Icons.Filled.Label, MaterialTheme.colorScheme.primary, "商机识别规则") { onNavigate(SettingsRoute.Detection) } }
                item { settingsRow(Icons.Filled.Schedule, AppColors.warning, "工作时间") { onNavigate(SettingsRoute.WorkSchedule) } }
                item { settingsRow(Icons.Filled.Notifications, AppColors.destructive, "通知设置") { onNavigate(SettingsRoute.Notifications) } }
            }

            item { HorizontalDivider(Modifier.padding(vertical = 8.dp)) }
            item {
                TextButton(onClick = { session.logout() }, modifier = Modifier.padding(horizontal = 16.dp)) {
                    Text("退出登录", color = AppColors.destructive)
                }
            }
        }
    }
}

@Composable
private fun userHeader(session: SessionStore) {
    Row(Modifier.fillMaxWidth().padding(16.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Box(Modifier.size(48.dp).background(MaterialTheme.colorScheme.primary.copy(alpha = 0.15f), CircleShape), contentAlignment = Alignment.Center) {
            Text(session.currentUser?.displayName?.take(1) ?: "我", style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary)
        }
        Column(Modifier.weight(1f)) {
            Text(session.currentUser?.displayName ?: "—", style = MaterialTheme.typography.titleMedium)
            Text(session.currentUser?.email ?: "", style = MaterialTheme.typography.labelSmall, color = AppColors.muted)
        }
    }
}

@Composable
private fun sectionHeader(title: String) {
    Text(title, style = MaterialTheme.typography.labelMedium, color = AppColors.muted, modifier = Modifier.padding(start = 16.dp, top = 16.dp, bottom = 4.dp))
}

@Composable
private fun settingsRow(icon: ImageVector, tint: Color, title: String, onClick: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = tint)
        Spacer(Modifier.size(12.dp))
        Text(title, Modifier.weight(1f))
        Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null, tint = AppColors.muted)
    }
}
