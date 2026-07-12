package com.codeiy.im.feature.inbox

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.FilterList
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AssistChip
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
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
import com.codeiy.im.feature.subscription.SubscriptionScreen
import com.codeiy.im.model.FrontendOpportunityStatus
import com.codeiy.im.model.IMChannel
import com.codeiy.im.model.Opportunity
import com.codeiy.im.model.Priority
import com.codeiy.im.ui.relativeTime
import kotlin.coroutines.cancellation.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

class InboxViewModel(private val service: RadarApi) : ViewModel() {
    data class UiState(
        val items: List<Opportunity> = emptyList(),
        val statusFilter: FrontendOpportunityStatus? = null,
        val platformFilter: IMChannel? = null,
        val isLoading: Boolean = false,
        val isLoadingMore: Boolean = false,
        val canLoadMore: Boolean = false,
        val error: String? = null,
    )

    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state
    private val pageSize = 50

    // 单一在途请求：refresh 取消一切在途加载，loadMore 只在空闲时启动，避免筛选/轮询/分页互相竞态。
    private var loadJob: Job? = null

    /** 只更新筛选；请求由界面按筛选 key 的 LaunchedEffect 统一触发，避免双发。 */
    fun setFilters(status: FrontendOpportunityStatus?, platform: IMChannel?) {
        _state.value = _state.value.copy(statusFilter = status, platformFilter = platform)
    }

    /** 取消在途请求后重载第一页；返回 Job 供轮询 join，保证一轮完成后再等下一轮。 */
    fun refresh(): Job {
        loadJob?.cancel()
        return viewModelScope.launch { load(reset = true) }.also { loadJob = it }
    }

    fun loadMoreIfNeeded(item: Opportunity) {
        val current = _state.value
        if (current.canLoadMore && loadJob?.isActive != true && item.id == current.items.lastOrNull()?.id) {
            loadJob = viewModelScope.launch { load(reset = false) }
        }
    }

    private suspend fun load(reset: Boolean) {
        val requested = _state.value
        _state.value = requested.copy(isLoading = reset, isLoadingMore = !reset)
        try {
            val offset = if (reset) 0 else requested.items.size
            val page = api {
                service.opportunities(
                    status = requested.statusFilter?.let { RadarEnumNames.status(it) },
                    platform = requested.platformFilter?.let { RadarEnumNames.channel(it) },
                    limit = pageSize,
                    offset = offset,
                )
            }
            val current = _state.value
            // 写入前校验筛选未变：慢响应不得覆盖新筛选的结果。
            if (current.statusFilter != requested.statusFilter ||
                current.platformFilter != requested.platformFilter
            ) {
                return
            }
            _state.value = current.copy(
                items = if (reset) page else current.items + page,
                canLoadMore = page.size == pageSize,
                isLoading = false,
                isLoadingMore = false,
                error = null,
            )
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            _state.value = _state.value.copy(isLoading = false, isLoadingMore = false, error = e.message)
        }
    }
}

/** 枚举 → 后端查询参数（SerialName 值）。 */
object RadarEnumNames {
    fun status(value: FrontendOpportunityStatus): String = when (value) {
        FrontendOpportunityStatus.PENDING -> "pending"
        FrontendOpportunityStatus.REPLIED -> "replied"
        FrontendOpportunityStatus.IGNORED -> "ignored"
        FrontendOpportunityStatus.UNKNOWN -> "pending"
    }

    fun channel(value: IMChannel): String = when (value) {
        IMChannel.TELEGRAM -> "telegram"
        IMChannel.WECOM -> "wecom"
        IMChannel.UNKNOWN -> "telegram"
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun InboxScreen(session: SessionStore, onOpenOpportunity: (String) -> Unit) {
    val model: InboxViewModel = viewModel { InboxViewModel(session.api.service) }
    val state by model.state.collectAsState()
    var showChannelMenu by remember { mutableStateOf(false) }
    var showAccountMenu by remember { mutableStateOf(false) }
    var showSubscription by remember { mutableStateOf(false) }

    if (showSubscription) {
        SubscriptionScreen(session = session, onBack = { showSubscription = false })
        return
    }

    // 筛选变化重启本效应（取消上一轮）；每轮等 refresh 完成再延时，轮询不会堆叠请求。
    LaunchedEffect(state.statusFilter, state.platformFilter) {
        while (true) {
            model.refresh().join()
            delay(30_000)
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("商机收件箱") },
                actions = {
                    IconButton(onClick = { showAccountMenu = true }) {
                        Icon(Icons.Filled.AccountCircle, contentDescription = "账号")
                    }
                    DropdownMenu(expanded = showAccountMenu, onDismissRequest = { showAccountMenu = false }) {
                        DropdownMenuItem(
                            text = { Text(session.currentUser?.displayName ?: "我") },
                            onClick = {},
                            enabled = false,
                        )
                        DropdownMenuItem(text = { Text("套餐与用量") }, onClick = { showAccountMenu = false; showSubscription = true })
                        DropdownMenuItem(text = { Text("退出登录") }, onClick = { session.logout() })
                    }
                },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding)) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                FilterChip(
                    selected = state.statusFilter == null,
                    onClick = { model.setFilters(null, state.platformFilter) },
                    label = { Text("全部") },
                )
                listOf(
                    FrontendOpportunityStatus.PENDING,
                    FrontendOpportunityStatus.REPLIED,
                    FrontendOpportunityStatus.IGNORED,
                ).forEach { status ->
                    FilterChip(
                        selected = state.statusFilter == status,
                        onClick = { model.setFilters(status, state.platformFilter) },
                        label = { Text(status.label) },
                    )
                }
                Spacer(Modifier.width(0.dp))
                Box {
                    IconButton(onClick = { showChannelMenu = true }) {
                        Icon(Icons.Filled.FilterList, contentDescription = "渠道筛选")
                    }
                    DropdownMenu(expanded = showChannelMenu, onDismissRequest = { showChannelMenu = false }) {
                        DropdownMenuItem(text = { Text("全部渠道") }, onClick = {
                            model.setFilters(state.statusFilter, null); showChannelMenu = false
                        })
                        listOf(IMChannel.TELEGRAM, IMChannel.WECOM).forEach { channel ->
                            DropdownMenuItem(text = { Text(channel.label) }, onClick = {
                                model.setFilters(state.statusFilter, channel); showChannelMenu = false
                            })
                        }
                    }
                }
            }
            HorizontalDivider()

            PullToRefreshBox(
                isRefreshing = state.isLoading && state.items.isNotEmpty(),
                onRefresh = { model.refresh() },
                modifier = Modifier.fillMaxSize(),
            ) {
                when {
                    state.error != null && state.items.isEmpty() -> Column(
                        Modifier.fillMaxSize().padding(24.dp),
                        verticalArrangement = Arrangement.Center,
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        Text(state.error ?: "", color = MaterialTheme.colorScheme.error)
                        Text(
                            "下拉重试",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    state.items.isEmpty() && state.isLoading -> Box(
                        Modifier.fillMaxSize(),
                        contentAlignment = Alignment.Center,
                    ) { CircularProgressIndicator() }
                    state.items.isEmpty() && !state.isLoading -> Box(
                        Modifier.fillMaxSize(),
                        contentAlignment = Alignment.Center,
                    ) { Text("暂无商机", color = MaterialTheme.colorScheme.onSurfaceVariant) }
                    else -> Column(Modifier.fillMaxSize()) {
                        // 已有内容时的刷新失败提示：点按重试。
                        if (state.error != null) {
                            Text(
                                "刷新失败：${state.error}（点按重试）",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clickable { model.refresh() }
                                    .padding(horizontal = 12.dp, vertical = 6.dp),
                            )
                        }
                        LazyColumn(Modifier.fillMaxSize()) {
                            items(state.items, key = { it.id }) { opportunity ->
                                InboxRow(opportunity) { onOpenOpportunity(opportunity.id) }
                                HorizontalDivider()
                                LaunchedEffect(opportunity.id) { model.loadMoreIfNeeded(opportunity) }
                            }
                            if (state.isLoadingMore) {
                                item {
                                    Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
                                        CircularProgressIndicator()
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun InboxRow(opportunity: Opportunity, onClick: () -> Unit) {
    Column(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (opportunity.attentionRequired) {
                Icon(
                    Icons.Filled.Warning,
                    contentDescription = "重点关注",
                    tint = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(end = 4.dp),
                )
            }
            Text(
                opportunity.contactName,
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.weight(1f),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                relativeTime(opportunity.updatedAt),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Text(
            opportunity.summary,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
            AssistChip(onClick = {}, label = { Text(opportunity.platform.label) }, enabled = false)
            AssistChip(onClick = {}, label = { Text(opportunity.internalStatus.label) }, enabled = false)
            if (opportunity.priority == Priority.HIGH || opportunity.priority == Priority.URGENT) {
                AssistChip(onClick = {}, label = { Text(opportunity.priority.label) }, enabled = false)
            }
            opportunity.groupName?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}
