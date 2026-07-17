package com.codeiy.im.feature.opportunity

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import com.codeiy.im.core.auth.SessionStore
import com.codeiy.im.core.network.RadarApi
import com.codeiy.im.core.network.api
import com.codeiy.im.model.ChatMessage
import com.codeiy.im.model.ManualReplyRequest
import com.codeiy.im.model.MessageSource
import com.codeiy.im.model.Opportunity
import com.codeiy.im.model.OpportunityStatus
import com.codeiy.im.model.OpportunityStatusUpdate
import com.codeiy.im.model.ReplyTemplate
import com.codeiy.im.model.displayText
import com.codeiy.im.ui.shortDateTime
import kotlin.coroutines.cancellation.CancellationException
import java.util.UUID
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

class OpportunityDetailModel(
    private val service: RadarApi,
    private val opportunityId: String,
    private val operatorName: () -> String?,
) : ViewModel() {
    data class UiState(
        val detail: Opportunity? = null,
        val messages: List<ChatMessage> = emptyList(),
        val templates: List<ReplyTemplate> = emptyList(),
        val templatesLoaded: Boolean = false,
        val isLoadingTemplates: Boolean = false,
        val replyText: String = "",
        val isLoading: Boolean = false,
        val isSending: Boolean = false,
        val isDrafting: Boolean = false,
        val error: String? = null,
    )

    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state

    private var loadJob: Job? = null
    private var pendingReply: Pair<String, String>? = null

    private val operatorId: String
        get() = operatorName()?.ifBlank { null } ?: "operator"

    fun load() {
        loadJob?.cancel()
        _state.value = _state.value.copy(isLoading = true)
        loadJob = viewModelScope.launch {
            try {
                coroutineScope {
                    val detail = async { api { service.opportunity(opportunityId) } }
                    val messages = async { api { service.messages(opportunityId) } }
                    _state.value = _state.value.copy(
                        detail = detail.await(),
                        messages = messages.await(),
                        isLoading = false,
                        error = null,
                    )
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.value = _state.value.copy(isLoading = false, error = e.message)
            }
        }
    }

    fun setReplyText(value: String) {
        _state.value = _state.value.copy(replyText = value)
    }

    /** 发送失败只报错、不改状态（验收：不得伪造已回复）。 */
    fun sendReply() {
        val text = _state.value.replyText.trim()
        if (text.isEmpty()) return
        val request = pendingReply?.takeIf { it.first == text }
            ?: (text to UUID.randomUUID().toString())
        pendingReply = request
        _state.value = _state.value.copy(isSending = true)
        viewModelScope.launch {
            try {
                val updated = api {
                    service.manualReply(
                        opportunityId,
                        request.second,
                        ManualReplyRequest(text, operatorId),
                    )
                }
                val messages = runCatching {
                    api { service.messages(opportunityId) }
                }.getOrDefault(_state.value.messages)
                _state.value = _state.value.copy(
                    detail = updated,
                    messages = messages,
                    replyText = "",
                    isSending = false,
                )
                pendingReply = null
            } catch (e: Exception) {
                _state.value = _state.value.copy(isSending = false, error = e.message)
            }
        }
    }

    /** 额度耗尽等错误直接展示后端 detail（fail-closed，验收要求明确提示）。 */
    fun generateDraft() {
        _state.value = _state.value.copy(isDrafting = true)
        viewModelScope.launch {
            try {
                val draft = api { service.aiDraft(opportunityId) }
                _state.value = _state.value.copy(replyText = draft.draft, isDrafting = false)
            } catch (e: Exception) {
                _state.value = _state.value.copy(isDrafting = false, error = e.message)
            }
        }
    }

    /** 非法状态迁移由后端 409 拒绝，错误原样展示（验收）。 */
    fun setStatus(status: OpportunityStatus) {
        viewModelScope.launch {
            try {
                val updated = api {
                    service.updateStatus(opportunityId, OpportunityStatusUpdate(status))
                }
                _state.value = _state.value.copy(detail = updated)
            } catch (e: Exception) {
                _state.value = _state.value.copy(error = e.message)
            }
        }
    }

    fun claim() {
        viewModelScope.launch {
            try {
                val updated = api { service.claim(opportunityId, operatorId) }
                _state.value = _state.value.copy(detail = updated)
            } catch (e: Exception) {
                _state.value = _state.value.copy(error = e.message)
            }
        }
    }

    /** 只加载一次；失败置 templatesLoaded=false，Sheet 内可重试。 */
    fun loadTemplatesIfNeeded() {
        val current = _state.value
        if (current.templatesLoaded || current.isLoadingTemplates) return
        _state.value = current.copy(isLoadingTemplates = true)
        viewModelScope.launch {
            val templates = runCatching { api { service.templates() } }
            _state.value = _state.value.copy(
                templates = templates.getOrDefault(emptyList()),
                templatesLoaded = templates.isSuccess,
                isLoadingTemplates = false,
            )
        }
    }

    fun dismissError() {
        _state.value = _state.value.copy(error = null)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OpportunityDetailScreen(session: SessionStore, opportunityId: String, onBack: () -> Unit) {
    val model: OpportunityDetailModel = viewModel(key = opportunityId) {
        OpportunityDetailModel(session.api.service, opportunityId) { session.currentUser?.displayName }
    }
    val state by model.state.collectAsState()
    var showStatusMenu by remember { mutableStateOf(false) }
    var showTemplates by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { model.load() }
    // 副作用放 LaunchedEffect，不在重组期间发请求。
    LaunchedEffect(showTemplates) { if (showTemplates) model.loadTemplatesIfNeeded() }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("商机详情") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    IconButton(onClick = { showStatusMenu = true }) {
                        Icon(Icons.Filled.MoreVert, contentDescription = "状态操作")
                    }
                    DropdownMenu(expanded = showStatusMenu, onDismissRequest = { showStatusMenu = false }) {
                        DropdownMenuItem(text = { Text("认领给我") }, onClick = {
                            model.claim(); showStatusMenu = false
                        })
                        HorizontalDivider()
                        DropdownMenuItem(text = { Text("标记跟进") }, onClick = {
                            model.setStatus(OpportunityStatus.FOLLOWING); showStatusMenu = false
                        })
                        DropdownMenuItem(text = { Text("忽略") }, onClick = {
                            model.setStatus(OpportunityStatus.IGNORED); showStatusMenu = false
                        })
                        DropdownMenuItem(
                            text = { Text("关闭", color = MaterialTheme.colorScheme.error) },
                            onClick = { model.setStatus(OpportunityStatus.CLOSED); showStatusMenu = false },
                        )
                    }
                },
            )
        },
        // 详情未加载成功前不给回复入口。
        bottomBar = {
            if (state.detail != null) {
                ReplyBar(model, state, onShowTemplates = { showTemplates = true })
            }
        },
    ) { padding ->
        when {
            state.detail == null && state.isLoading -> Box(
                Modifier.fillMaxSize().padding(padding),
                contentAlignment = Alignment.Center,
            ) { CircularProgressIndicator() }
            state.detail == null -> Column(
                Modifier.fillMaxSize().padding(padding).padding(24.dp),
                verticalArrangement = Arrangement.Center,
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(state.error ?: "加载失败", color = MaterialTheme.colorScheme.error)
                Spacer(Modifier.height(12.dp))
                TextButton(onClick = { model.dismissError(); model.load() }) { Text("重试") }
            }
            else -> PullToRefreshBox(
                isRefreshing = state.isLoading,
                onRefresh = { model.load() },
                modifier = Modifier.padding(padding).fillMaxSize(),
            ) {
                DetailContent(state)
            }
        }
    }

    if (showTemplates) {
        ModalBottomSheet(onDismissRequest = { showTemplates = false }) {
            TemplatePicker(
                templates = state.templates,
                isLoading = state.isLoadingTemplates,
                onRetry = { model.loadTemplatesIfNeeded() },
            ) { template ->
                model.setReplyText(template.content)
                showTemplates = false
            }
        }
    }

    state.error?.let { message ->
        AlertDialog(
            onDismissRequest = { model.dismissError() },
            confirmButton = { TextButton(onClick = { model.dismissError() }) { Text("好") } },
            title = { Text("操作失败") },
            text = { Text(message) },
        )
    }
}

@Composable
private fun DetailContent(state: OpportunityDetailModel.UiState, modifier: Modifier = Modifier) {
    val detail = state.detail ?: return
    LazyColumn(modifier = modifier.fillMaxSize(), contentPadding = androidx.compose.foundation.layout.PaddingValues(12.dp)) {
        item { SectionTitle("概要") }
        item { LabeledRow("联系人", detail.contactName) }
        item { LabeledRow("渠道", detail.platform.label) }
        detail.groupName?.let { item { LabeledRow("群组", it) } }
        item { LabeledRow("状态", detail.internalStatus.label) }
        item { LabeledRow("优先级", detail.priority.label) }
        item { LabeledRow("置信度", "${(detail.confidenceScore * 100).toInt()}%") }
        if (detail.matchedKeywords.isNotEmpty()) {
            item { LabeledRow("命中关键词", detail.matchedKeywords.joinToString("、")) }
        }
        detail.detectionReason?.let { item { LabeledRow("识别依据", it) } }

        item { SectionTitle("Agent 发现（${detail.agentAnalysisStatus.label}）") }
        if (detail.attentionRequired) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Filled.Warning, contentDescription = null, tint = MaterialTheme.colorScheme.error)
                    Spacer(Modifier.width(6.dp))
                    Text("重大商机，需要关注", color = MaterialTheme.colorScheme.error)
                }
            }
        }
        detail.agentAnalysisError?.let {
            item { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
        }
        items(detail.linkVerification.entries.sortedBy { it.key }) { (key, value) ->
            LabeledRow("链接核验 · $key", value.displayText())
        }
        items(detail.extractedContacts.entries.sortedBy { it.key }) { (key, value) ->
            LabeledRow("联系方式 · $key", value.displayText())
        }
        items(detail.agentActions) { action ->
            Column(Modifier.padding(vertical = 4.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(action.actionType.label, style = MaterialTheme.typography.titleSmall)
                    if (action.requiresApproval) {
                        AssistChip(onClick = {}, label = { Text("需人工批准") }, enabled = false)
                    }
                }
                Text(action.reason, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                action.draft?.takeIf { it.isNotEmpty() }?.let {
                    Surface(shape = RoundedCornerShape(6.dp), color = MaterialTheme.colorScheme.surfaceVariant) {
                        Text(it, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(6.dp))
                    }
                }
            }
        }
        if (detail.agentActions.isEmpty() && detail.linkVerification.isEmpty() &&
            detail.extractedContacts.isEmpty() && !detail.attentionRequired
        ) {
            item { Text("暂无 Agent 发现", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }

        item { SectionTitle("消息历史") }
        if (state.messages.isEmpty()) {
            item { Text("暂无消息", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
        items(state.messages, key = { it.id }) { message -> MessageBubble(message) }
    }
}

@Composable
private fun SectionTitle(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.titleMedium,
        modifier = Modifier.padding(top = 16.dp, bottom = 8.dp),
    )
}

@Composable
private fun LabeledRow(label: String, value: String) {
    Row(Modifier.fillMaxWidth().padding(vertical = 3.dp)) {
        Text(
            label,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.width(120.dp),
        )
        Text(value, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
    }
}

@Composable
private fun MessageBubble(message: ChatMessage) {
    val fromContact = message.isFromContact
    Column(
        modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp),
        horizontalAlignment = if (fromContact) Alignment.Start else Alignment.End,
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(message.senderName, style = MaterialTheme.typography.labelMedium)
            if (message.source == MessageSource.AI) {
                AssistChip(onClick = {}, label = { Text("AI") }, enabled = false)
            }
            Text(
                shortDateTime(message.sentAt),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Box(
            Modifier
                .background(
                    if (fromContact) MaterialTheme.colorScheme.surfaceVariant
                    else MaterialTheme.colorScheme.primaryContainer,
                    RoundedCornerShape(10.dp),
                )
                .padding(8.dp),
        ) {
            Text(message.content, style = MaterialTheme.typography.bodyMedium)
        }
    }
}

@Composable
private fun ReplyBar(
    model: OpportunityDetailModel,
    state: OpportunityDetailModel.UiState,
    onShowTemplates: () -> Unit,
) {
    Surface(tonalElevation = 3.dp) {
        Column(Modifier.padding(12.dp)) {
            val existingDraft = state.detail?.aiReplyDraft
            if (!existingDraft.isNullOrEmpty() && state.replyText.isEmpty()) {
                Text(
                    "使用已有 AI 草稿",
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.primary,
                    modifier = Modifier
                        .clickable { model.setReplyText(existingDraft) }
                        .padding(bottom = 6.dp),
                )
            }
            Row(verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                OutlinedTextField(
                    value = state.replyText,
                    onValueChange = model::setReplyText,
                    placeholder = { Text("输入回复…") },
                    modifier = Modifier.weight(1f),
                    maxLines = 4,
                )
                IconButton(onClick = onShowTemplates) {
                    Icon(Icons.Filled.Description, contentDescription = "回复模板")
                }
                IconButton(onClick = { model.generateDraft() }, enabled = !state.isDrafting) {
                    if (state.isDrafting) {
                        CircularProgressIndicator(Modifier.height(20.dp))
                    } else {
                        Icon(Icons.Filled.AutoAwesome, contentDescription = "AI 草稿")
                    }
                }
                IconButton(
                    onClick = { model.sendReply() },
                    enabled = !state.isSending && state.replyText.isNotBlank(),
                ) {
                    if (state.isSending) {
                        CircularProgressIndicator(Modifier.height(20.dp))
                    } else {
                        Icon(Icons.AutoMirrored.Filled.Send, contentDescription = "发送")
                    }
                }
            }
        }
    }
}

@Composable
private fun TemplatePicker(
    templates: List<ReplyTemplate>,
    isLoading: Boolean,
    onRetry: () -> Unit,
    onPick: (ReplyTemplate) -> Unit,
) {
    Column {
        Text(
            "回复模板",
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        HorizontalDivider()
        when {
            isLoading -> Box(Modifier.fillMaxWidth().padding(32.dp), contentAlignment = Alignment.Center) {
                CircularProgressIndicator()
            }
            templates.isEmpty() -> Column(
                Modifier.fillMaxWidth().padding(32.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text("暂无模板", color = MaterialTheme.colorScheme.onSurfaceVariant)
                TextButton(onClick = onRetry) { Text("重新加载") }
            }
            else -> LazyColumn(contentPadding = androidx.compose.foundation.layout.PaddingValues(bottom = 24.dp)) {
                items(templates, key = { it.id }) { template ->
                    Column(
                        Modifier.fillMaxWidth().clickable { onPick(template) }.padding(horizontal = 16.dp, vertical = 10.dp),
                    ) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            Text(template.title, style = MaterialTheme.typography.titleSmall)
                            AssistChip(onClick = {}, label = { Text(template.category) }, enabled = false)
                        }
                        Text(
                            template.content,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 3,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                    HorizontalDivider()
                }
            }
        }
    }
}
