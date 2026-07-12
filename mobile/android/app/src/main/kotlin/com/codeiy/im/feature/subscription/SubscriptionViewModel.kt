package com.codeiy.im.feature.subscription

import android.app.Activity
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.codeiy.im.core.auth.SessionStore
import com.codeiy.im.core.billing.BillingPackageOption
import com.codeiy.im.core.billing.BillingPurchaseResult
import com.codeiy.im.core.billing.BillingService
import com.codeiy.im.core.network.api
import com.codeiy.im.model.BillingInterval
import com.codeiy.im.model.PlanCode
import com.codeiy.im.model.SubscriptionCatalogPlan
import com.codeiy.im.model.SubscriptionManagement
import com.codeiy.im.model.SubscriptionUsage
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

interface SubscriptionBackend {
    suspend fun usage(): SubscriptionUsage
    suspend fun catalog(): List<SubscriptionCatalogPlan>
    suspend fun sync(): SubscriptionUsage
    suspend fun management(): SubscriptionManagement
}

class SessionSubscriptionBackend(private val session: SessionStore) : SubscriptionBackend {
    override suspend fun usage() = api { session.api.service.subscription() }
    override suspend fun catalog() = api { session.api.service.subscriptionCatalog() }
    override suspend fun sync() = api { session.api.service.syncSubscription() }
    override suspend fun management() = api { session.api.service.subscriptionManagement() }
}

data class SubscriptionUiState(
    val usage: SubscriptionUsage? = null,
    val catalog: List<SubscriptionCatalogPlan> = emptyList(),
    val management: SubscriptionManagement? = null,
    val packages: List<BillingPackageOption> = emptyList(),
    val interval: BillingInterval = BillingInterval.MONTHLY,
    val loading: Boolean = false,
    val busyPackageId: String? = null,
    val restoring: Boolean = false,
    val message: String? = null,
    val error: String? = null,
)

class SubscriptionViewModel(
    private val userId: String,
    private val backend: SubscriptionBackend,
    private val billing: BillingService,
) : ViewModel() {
    private val _state = MutableStateFlow(SubscriptionUiState())
    val state: StateFlow<SubscriptionUiState> = _state

    fun setInterval(interval: BillingInterval) = _state.update { it.copy(interval = interval) }

    fun load() = viewModelScope.launch {
        _state.update { it.copy(loading = true, error = null) }
        runCatching {
            val usage = async { backend.usage() }
            val catalog = async { backend.catalog() }
            val management = async { backend.management() }
            val packages = if (billing.isConfigured) {
                billing.identify(userId)
                billing.fetchPackages()
            } else emptyList()
            _state.update { it.copy(usage = usage.await(), catalog = catalog.await(), management = management.await(), packages = packages) }
        }.onFailure { error -> _state.update { it.copy(error = error.message ?: "无法加载订阅信息") } }
        _state.update { it.copy(loading = false) }
    }

    fun packageFor(plan: PlanCode): BillingPackageOption? = state.value.packages.firstOrNull {
        it.planCode == plan && it.interval == state.value.interval
    }

    fun purchase(activity: Activity, option: BillingPackageOption) = viewModelScope.launch {
        if (state.value.usage?.planCode != PlanCode.FREE || !billing.isConfigured) return@launch
        _state.update { it.copy(busyPackageId = option.id, message = null, error = null) }
        runCatching { billing.purchase(activity, option.id) }
            .onSuccess { result ->
                if (result == BillingPurchaseResult.Cancelled) {
                    _state.update { it.copy(message = "已取消购买，未产生套餐变更。") }
                } else {
                    _state.update { it.copy(message = "支付已完成，正在确认订阅权益。") }
                    val usage = backend.sync()
                    val management = backend.management()
                    _state.update { it.copy(usage = usage, management = management, message = if (usage.planCode == option.planCode) "订阅权益已生效。" else it.message) }
                }
            }
            .onFailure { error -> _state.update { it.copy(error = error.message ?: "购买失败") } }
        _state.update { it.copy(busyPackageId = null) }
    }

    fun restore() = viewModelScope.launch {
        if (!billing.isConfigured) { _state.update { it.copy(error = "支付尚未配置") }; return@launch }
        _state.update { it.copy(restoring = true, error = null) }
        runCatching {
            billing.identify(userId)
            billing.restorePurchases()
            backend.sync() to backend.management()
        }.onSuccess { (usage, management) ->
            _state.update { it.copy(usage = usage, management = management, message = "购买记录已同步。") }
        }.onFailure { error -> _state.update { it.copy(error = error.message ?: "恢复购买失败") } }
        _state.update { it.copy(restoring = false) }
    }
}
