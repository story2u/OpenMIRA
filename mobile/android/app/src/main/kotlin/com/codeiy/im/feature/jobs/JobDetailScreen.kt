package com.codeiy.im.feature.jobs

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.codeiy.im.core.auth.SessionStore
import com.codeiy.im.model.JobFeedbackType
import com.codeiy.im.model.JobOpportunityDetail
import com.codeiy.im.ui.theme.AppBadge
import com.codeiy.im.ui.theme.AppCard
import com.codeiy.im.ui.theme.AppColors

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun JobDetailScreen(
    session: SessionStore,
    opportunityId: String,
    profileId: String?,
    onBack: () -> Unit,
) {
    val model: JobDetailViewModel = viewModel(key = "$opportunityId:$profileId") {
        JobDetailViewModel(session.api.service, opportunityId, profileId)
    }
    val state by model.state.collectAsStateWithLifecycle()
    LaunchedEffect(opportunityId, profileId) { model.load() }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("工作机会详情") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
            )
        },
    ) { padding ->
        when {
            state.loading && state.detail == null -> Box(Modifier.padding(padding).fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
            state.detail == null -> Box(Modifier.padding(padding).fillMaxSize(), contentAlignment = Alignment.Center) { Text(state.error ?: "职位不存在") }
            else -> JobDetailContent(
                detail = state.detail!!,
                feedbackSent = state.feedbackSent,
                onFeedback = model::feedback,
                modifier = Modifier.padding(padding),
            )
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun JobDetailContent(
    detail: JobOpportunityDetail,
    feedbackSent: JobFeedbackType?,
    onFeedback: (JobFeedbackType) -> Unit,
    modifier: Modifier = Modifier,
) {
    val uriHandler = LocalUriHandler.current
    Column(
        modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(detail.jobTitle, style = MaterialTheme.typography.headlineSmall)
            Text(detail.companyName ?: "公司未说明", color = AppColors.muted)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                detail.match?.let { AppBadge("匹配 ${it.matchScore}", MaterialTheme.colorScheme.primary) }
                AppBadge(detail.workMode.label, AppColors.success)
                AppBadge(detail.employmentType.label, AppColors.ai)
                AppBadge(detail.seniority.label, AppColors.telegram)
            }
            Text(
                listOfNotNull(detail.locationText, detail.salaryRaw, detail.postedAt.take(10)).joinToString(" · "),
                style = MaterialTheme.typography.bodyMedium,
            )
            detail.applicationUrl?.takeIf(::isSafeWebUrl)?.let { url ->
                Button(onClick = { uriHandler.openUri(url) }) { Text("前往投递") }
            }
        }

        detail.match?.let { match ->
            DetailSection("匹配分析") {
                ReasonGroup("符合", match.matchedReasons, AppColors.success)
                ReasonGroup("不符合", match.mismatchReasons, AppColors.destructive)
                ReasonGroup("招聘信息未说明", match.unknownConstraints, AppColors.warning)
                if (match.scoreBreakdown.isNotEmpty()) {
                    Text("分数构成", style = MaterialTheme.typography.labelLarge)
                    match.scoreBreakdown.forEach { (name, score) -> Text("$name  $score", style = MaterialTheme.typography.bodySmall) }
                }
            }
        }

        DetailSection("核心要求") {
            detail.requirementsSummary?.let { Text(it) }
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                detail.requiredSkills.forEach { AppBadge(it, AppColors.ai) }
            }
            Fact("经验", experienceText(detail))
            Fact("学历", detail.degreeLevel ?: if (detail.degreeRequired == false) "未要求" else "未说明")
            Fact("英语", detail.englishLevel ?: "未说明")
            Fact("签证支持", detail.visaSponsorship.toReadable())
            Fact("搬迁支持", detail.relocationSupport.toReadable())
        }

        if (detail.ageRequirementPresent || detail.complianceFlags.isNotEmpty()) {
            DetailSection("合规提示") {
                if (detail.ageRequirementPresent) {
                    Text(
                        "招聘原文包含年龄限制：${detail.ageRequirementText ?: "具体内容见原文"}。该条件可能涉及就业歧视，系统不会将用户年龄用于推荐计算。",
                        color = AppColors.warning,
                    )
                }
                detail.complianceFlags.filterNot { it.contains("age", ignoreCase = true) }.forEach {
                    Text("需人工核验：$it", color = AppColors.warning)
                }
            }
        }

        DetailSection("来源信息") {
            Fact("平台", detail.sourceChannel.label)
            Fact("群组", detail.sourceChatName ?: "私有来源")
            Fact("发布者", detail.sourceAuthorName ?: "未说明")
            Fact("发布时间", detail.postedAt)
            Fact("来源可信度", "${(detail.sourceReliabilityScore * 100).toInt()}%")
            detail.sourceMessageUrl?.takeIf(::isSafeWebUrl)?.let { url ->
                TextButton(onClick = { uriHandler.openUri(url) }) { Text("在原平台查看") }
            } ?: Text("该消息来自私有群组，只能在原平台查看。", color = AppColors.muted)
            if (detail.sources.size > 1) Text("发现于 ${detail.sources.size} 个来源")
            if (detail.conflictingSourceData) Text("不同来源的信息存在冲突，请以原文为准。", color = AppColors.warning)
        }

        DetailSection("原始信息与证据") {
            Text(detail.rawExcerpt)
            detail.fieldEvidence.forEach { (field, evidence) ->
                Text("$field：$evidence", style = MaterialTheme.typography.bodySmall, color = AppColors.muted)
            }
            if (detail.missingFields.isNotEmpty()) Text("缺失字段：${detail.missingFields.joinToString("、")}", color = AppColors.muted)
        }

        DetailSection("反馈") {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                JobFeedbackType.entries.filter { it != JobFeedbackType.UNKNOWN }.forEach { type ->
                    TextButton(onClick = { onFeedback(type) }, enabled = feedbackSent != type) {
                        Text(if (feedbackSent == type) "已反馈：${type.label}" else type.label)
                    }
                }
            }
        }
    }
}

@Composable
private fun DetailSection(title: String, content: @Composable () -> Unit) {
    AppCard(Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium)
            content()
        }
    }
}

@Composable
private fun ReasonGroup(title: String, reasons: List<String>, color: androidx.compose.ui.graphics.Color) {
    if (reasons.isEmpty()) return
    Text(title, style = MaterialTheme.typography.labelLarge, color = color)
    reasons.forEach { Text("• $it", style = MaterialTheme.typography.bodySmall) }
}

@Composable
private fun Fact(label: String, value: String) {
    Row(Modifier.fillMaxWidth()) {
        Text(label, color = AppColors.muted)
        Text(value, Modifier.weight(1f).padding(start = 16.dp), textAlign = TextAlign.End)
    }
}

private fun experienceText(detail: JobOpportunityDetail): String = when {
    detail.minimumYearsExperience != null && detail.maximumYearsExperience != null -> "${detail.minimumYearsExperience}-${detail.maximumYearsExperience} 年"
    detail.minimumYearsExperience != null -> "至少 ${detail.minimumYearsExperience} 年"
    else -> "未说明"
}

private fun Boolean?.toReadable() = when (this) {
    true -> "是"
    false -> "否"
    null -> "未说明"
}

private fun isSafeWebUrl(value: String): Boolean = value.startsWith("https://") || value.startsWith("http://")
