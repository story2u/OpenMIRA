package com.codeiy.im.feature.jobs

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.codeiy.im.core.auth.SessionStore
import com.codeiy.im.model.JobSearchProfile
import com.codeiy.im.model.JobSearchProfilePreview
import com.codeiy.im.model.JobSearchProfileWrite
import com.codeiy.im.ui.theme.AppBadge
import com.codeiy.im.ui.theme.AppCard
import com.codeiy.im.ui.theme.AppColors

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun JobSearchProfilesScreen(session: SessionStore, onBack: () -> Unit) {
    val model: JobProfilesViewModel = viewModel { JobProfilesViewModel(session.api.service) }
    val state by model.state.collectAsStateWithLifecycle()
    var naturalText by remember { mutableStateOf("") }
    var editing by remember { mutableStateOf<JobSearchProfile?>(null) }
    var showEditor by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { model.load() }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("求职档案") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
            )
        },
    ) { padding ->
        LazyColumn(
            Modifier.padding(padding).fillMaxSize(),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("用一句话描述求职目标", style = MaterialTheme.typography.titleMedium)
                    Text("Pi Agent 只生成结构化预览，确认前不会修改已有档案。", style = MaterialTheme.typography.bodySmall, color = AppColors.muted)
                    OutlinedTextField(
                        value = naturalText,
                        onValueChange = { naturalText = it },
                        label = { Text("例如：远程 Python 后端，欧洲时区，年薪至少 8 万美元") },
                        minLines = 3,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Button(
                        onClick = { model.parse(naturalText) },
                        enabled = naturalText.trim().length >= 5 && !state.parsing,
                    ) { Text(if (state.parsing) "正在解析" else "生成预览") }
                }
            }
            if (state.error != null) item { Text(state.error.orEmpty(), color = AppColors.destructive) }
            item {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                    Text("我的档案", style = MaterialTheme.typography.titleMedium)
                    OutlinedButton(onClick = { editing = null; showEditor = true }) { Text("新建") }
                }
            }
            items(state.profiles.size, key = { state.profiles[it].id }) { index ->
                val profile = state.profiles[index]
                ProfileCard(
                    profile = profile,
                    onEdit = { editing = profile; showEditor = true },
                    onDelete = { model.delete(profile) },
                )
            }
        }
    }

    state.preview?.let { preview ->
        PreviewDialog(
            preview = preview,
            saving = state.saving,
            onConfirm = model::savePreview,
            onDismiss = model::discardPreview,
        )
    }
    if (showEditor) {
        ProfileEditorSheet(
            profile = editing,
            onDismiss = { showEditor = false },
            onSave = { model.save(editing, it); showEditor = false },
        )
    }
}

@Composable
private fun ProfileCard(profile: JobSearchProfile, onEdit: () -> Unit, onDelete: () -> Unit) {
    AppCard(Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                Text(profile.name, style = MaterialTheme.typography.titleMedium)
                if (profile.isDefault) AppBadge("默认", MaterialTheme.colorScheme.primary)
            }
            Text(
                "目标：${profile.targetRoles.ifEmpty { listOf("未设置") }.joinToString("、")}",
                style = MaterialTheme.typography.bodySmall,
            )
            Text("技能：${profile.candidateSkills.ifEmpty { listOf("未设置") }.take(8).joinToString("、")}", style = MaterialTheme.typography.bodySmall)
            Text("最低匹配分 ${profile.minimumMatchScore}", style = MaterialTheme.typography.labelSmall, color = AppColors.muted)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                TextButton(onClick = onEdit) { Text("编辑") }
                TextButton(onClick = onDelete) { Text("删除", color = AppColors.destructive) }
            }
        }
    }
}

@Composable
private fun PreviewDialog(
    preview: JobSearchProfilePreview,
    saving: Boolean,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("确认求职档案") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text("名称：${preview.name}")
                Text("目标岗位：${preview.targetRoles.joinToString("、").ifEmpty { "未设置" }}")
                Text("技能：${preview.candidateSkills.joinToString("、").ifEmpty { "未设置" }}")
                Text("地点：${(preview.preferredCountries + preview.preferredCities).joinToString("、").ifEmpty { "未设置" }}")
                Text("工作模式：${preview.workModes.joinToString("、") { it.label }.ifEmpty { "未设置" }}")
                Text("薪资：${preview.minimumSalary?.let { "${preview.salaryCurrency.orEmpty()} $it" } ?: "未设置"}")
                Text("请核对以上字段。保存后才会用于确定性匹配。", color = AppColors.warning)
            }
        },
        confirmButton = { Button(onClick = onConfirm, enabled = !saving) { Text(if (saving) "保存中" else "确认并保存") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ProfileEditorSheet(
    profile: JobSearchProfile?,
    onDismiss: () -> Unit,
    onSave: (JobSearchProfileWrite) -> Unit,
) {
    var body by remember(profile?.id) { mutableStateOf(profile?.toWrite() ?: JobSearchProfileWrite(name = "")) }
    var roles by remember(profile?.id) { mutableStateOf(body.targetRoles.joinToString(", ")) }
    var skills by remember(profile?.id) { mutableStateOf(body.candidateSkills.joinToString(", ")) }
    var cities by remember(profile?.id) { mutableStateOf(body.preferredCities.joinToString(", ")) }
    var excluded by remember(profile?.id) { mutableStateOf(body.excludedKeywords.joinToString(", ")) }
    ModalBottomSheet(onDismissRequest = onDismiss) {
        Column(
            Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 20.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(if (profile == null) "新建求职档案" else "编辑求职档案", style = MaterialTheme.typography.titleLarge)
            OutlinedTextField(body.name, { body = body.copy(name = it) }, label = { Text("档案名称") }, modifier = Modifier.fillMaxWidth())
            OutlinedTextField(roles, { roles = it }, label = { Text("目标岗位，逗号分隔") }, modifier = Modifier.fillMaxWidth())
            OutlinedTextField(skills, { skills = it }, label = { Text("技能，逗号分隔") }, modifier = Modifier.fillMaxWidth())
            OutlinedTextField(cities, { cities = it }, label = { Text("偏好城市，逗号分隔") }, modifier = Modifier.fillMaxWidth())
            OutlinedTextField(excluded, { excluded = it }, label = { Text("排除关键词，逗号分隔") }, modifier = Modifier.fillMaxWidth())
            OutlinedTextField(
                value = body.minimumMatchScore.toString(),
                onValueChange = { body = body.copy(minimumMatchScore = it.toIntOrNull()?.coerceIn(0, 100) ?: 0) },
                label = { Text("最低匹配分") },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth(),
            )
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.SpaceBetween) {
                Text("启用职位通知")
                Switch(body.notificationEnabled, { body = body.copy(notificationEnabled = it) })
            }
            Button(
                onClick = {
                    onSave(
                        body.copy(
                            targetRoles = splitValues(roles),
                            candidateSkills = splitValues(skills),
                            preferredCities = splitValues(cities),
                            excludedKeywords = splitValues(excluded),
                        ),
                    )
                },
                enabled = body.name.isNotBlank(),
                modifier = Modifier.align(Alignment.End),
            ) { Text("保存") }
        }
    }
}

private fun splitValues(value: String): List<String> = value
    .split(',', '，')
    .map(String::trim)
    .filter(String::isNotEmpty)
    .distinct()
