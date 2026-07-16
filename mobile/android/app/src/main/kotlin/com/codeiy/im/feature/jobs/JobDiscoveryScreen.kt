package com.codeiy.im.feature.jobs

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Tune
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Slider
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
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.codeiy.im.core.auth.SessionStore
import com.codeiy.im.model.IMChannel
import com.codeiy.im.model.JobEmploymentType
import com.codeiy.im.model.JobOpportunity
import com.codeiy.im.model.JobWorkMode
import com.codeiy.im.ui.theme.AppBadge
import com.codeiy.im.ui.theme.AppCard
import com.codeiy.im.ui.theme.AppColors

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun JobDiscoveryScreen(
    session: SessionStore,
    onOpenJob: (String, String?) -> Unit,
    onOpenProfiles: () -> Unit,
) {
    val model: JobDiscoveryViewModel = viewModel { JobDiscoveryViewModel(session.api.service) }
    val state by model.state.collectAsStateWithLifecycle()
    var filtersOpen by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { model.load() }
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("工作机会")
                        Text("${state.total} 个匹配职位", style = MaterialTheme.typography.labelSmall, color = AppColors.muted)
                    }
                },
                actions = {
                    IconButton(onClick = { filtersOpen = true }) { Icon(Icons.Filled.Tune, "筛选工作机会") }
                    IconButton(onClick = model::load) { Icon(Icons.Filled.Refresh, "刷新工作机会") }
                },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).fillMaxSize()) {
            ProfileSelector(
                state = state,
                onSelect = { model.applyFilters(state.filters.copy(profileId = it)) },
                onOpenProfiles = onOpenProfiles,
            )
            when {
                state.loading && state.items.isEmpty() -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
                state.error != null && state.items.isEmpty() -> ErrorState(state.error.orEmpty(), model::load)
                state.items.isEmpty() -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Text("尚未发现符合条件的工作机会", color = AppColors.muted)
                }
                else -> LazyVerticalGrid(
                    columns = GridCells.Adaptive(340.dp),
                    contentPadding = PaddingValues(12.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    items(state.items, key = { it.opportunityId }) { job ->
                        JobCard(job) { onOpenJob(job.opportunityId, state.filters.profileId) }
                    }
                }
            }
        }
    }
    if (filtersOpen) {
        JobFilterSheet(
            initial = state.filters,
            onDismiss = { filtersOpen = false },
            onApply = { model.applyFilters(it); filtersOpen = false },
        )
    }
}

@Composable
private fun ProfileSelector(
    state: JobDiscoveryViewModel.UiState,
    onSelect: (String?) -> Unit,
    onOpenProfiles: () -> Unit,
) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        state.profiles.take(3).forEach { profile ->
            FilterChip(
                selected = state.filters.profileId == profile.id,
                onClick = { onSelect(profile.id) },
                label = { Text(profile.name) },
            )
        }
        TextButton(onClick = onOpenProfiles) { Text(if (state.profiles.isEmpty()) "创建求职档案" else "管理档案") }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun JobCard(job: JobOpportunity, onClick: () -> Unit) {
    AppCard(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .semantics { contentDescription = "${job.jobTitle}，${job.companyName ?: "公司未说明"}" },
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(9.dp)) {
            Row(verticalAlignment = Alignment.Top, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Column(Modifier.weight(1f)) {
                    Text(job.jobTitle, style = MaterialTheme.typography.titleMedium)
                    Text(job.companyName ?: "公司未说明", color = AppColors.muted)
                }
                job.match?.let { AppBadge("匹配 ${it.matchScore}", MaterialTheme.colorScheme.primary) }
            }
            Text(
                listOfNotNull(job.locationText, job.workMode.label, job.salaryRaw).joinToString(" · ").ifEmpty { "地点与薪资未说明" },
                style = MaterialTheme.typography.bodySmall,
            )
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                job.requiredSkills.take(4).forEach { AppBadge(it, AppColors.ai) }
            }
            if (job.complianceFlags.isNotEmpty()) {
                Text("包含需核验的招聘条件", color = AppColors.warning, style = MaterialTheme.typography.labelSmall)
            }
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                Text("${job.sourceChannel.label} · ${job.sourceChatName ?: "私有来源"}", style = MaterialTheme.typography.labelSmall, color = AppColors.muted)
                Text(job.postedAt.take(10), style = MaterialTheme.typography.labelSmall, color = AppColors.muted)
            }
            if (job.sourceCount > 1) Text("发现于 ${job.sourceCount} 个来源", style = MaterialTheme.typography.labelSmall)
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun JobFilterSheet(initial: JobFilters, onDismiss: () -> Unit, onApply: (JobFilters) -> Unit) {
    var value by remember(initial) { mutableStateOf(initial) }
    ModalBottomSheet(onDismissRequest = onDismiss) {
        Column(
            Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text("筛选工作机会", style = MaterialTheme.typography.titleLarge)
            OutlinedTextField(
                value = value.query,
                onValueChange = { value = value.copy(query = it) },
                label = { Text("岗位、公司或技能") },
                modifier = Modifier.fillMaxWidth(),
            )
            Text("消息来源", style = MaterialTheme.typography.labelMedium)
            ChoiceRow(
                choices = listOf(null to "全部", IMChannel.TELEGRAM to "Telegram", IMChannel.WECOM to "企业微信"),
                selected = value.source,
                onSelect = { value = value.copy(source = it) },
            )
            Text("工作模式", style = MaterialTheme.typography.labelMedium)
            ChoiceRow(
                choices = listOf(null to "全部", JobWorkMode.REMOTE to "远程", JobWorkMode.HYBRID to "混合", JobWorkMode.ON_SITE to "现场"),
                selected = value.workMode,
                onSelect = { value = value.copy(workMode = it) },
            )
            Text("雇佣类型", style = MaterialTheme.typography.labelMedium)
            ChoiceRow(
                choices = listOf(null to "全部", JobEmploymentType.FULL_TIME to "全职", JobEmploymentType.CONTRACT to "合同", JobEmploymentType.INTERNSHIP to "实习"),
                selected = value.employmentType,
                onSelect = { value = value.copy(employmentType = it) },
            )
            Text("最低匹配分 ${value.minimumMatchScore}")
            Slider(
                value = value.minimumMatchScore.toFloat(),
                onValueChange = { value = value.copy(minimumMatchScore = it.toInt()) },
                valueRange = 0f..100f,
                steps = 9,
            )
            ToggleRow("排除过期职位", value.excludeExpired) { value = value.copy(excludeExpired = it) }
            ToggleRow("只看没有明确年龄限制的职位", value.excludeExplicitAgeRequirement) {
                value = value.copy(excludeExplicitAgeRequirement = it)
            }
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End)) {
                TextButton(onClick = { value = JobFilters(profileId = initial.profileId) }) { Text("重置") }
                Button(onClick = { onApply(value) }) { Text("应用筛选") }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun <T> ChoiceRow(choices: List<Pair<T?, String>>, selected: T?, onSelect: (T?) -> Unit) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        choices.forEach { (choice, label) ->
            FilterChip(selected = selected == choice, onClick = { onSelect(choice) }, label = { Text(label) })
        }
    }
}

@Composable
private fun ToggleRow(label: String, checked: Boolean, onChecked: (Boolean) -> Unit) {
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.SpaceBetween) {
        Text(label, Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = onChecked)
    }
}

@Composable
private fun ErrorState(message: String, retry: () -> Unit) {
    Column(Modifier.fillMaxSize(), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
        Text(message, color = AppColors.destructive)
        TextButton(onClick = retry) { Text("重试") }
    }
}
