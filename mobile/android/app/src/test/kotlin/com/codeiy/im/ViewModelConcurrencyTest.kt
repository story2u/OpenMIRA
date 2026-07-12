package com.codeiy.im

import com.codeiy.im.feature.inbox.InboxViewModel
import com.codeiy.im.feature.opportunity.OpportunityDetailModel
import com.codeiy.im.core.network.RadarApi
import com.codeiy.im.model.AIDraft
import com.codeiy.im.model.AuthToken
import com.codeiy.im.model.AuthUser
import com.codeiy.im.model.ChatMessage
import com.codeiy.im.model.FrontendOpportunityStatus
import com.codeiy.im.model.ManualReplyRequest
import com.codeiy.im.model.NativeLoginRequest
import com.codeiy.im.model.Opportunity
import com.codeiy.im.model.OpportunityStatusUpdate
import com.codeiy.im.model.PasswordLoginRequest
import com.codeiy.im.model.ReplyTemplate
import com.codeiy.im.model.SubscriptionCatalogPlan
import com.codeiy.im.model.SubscriptionManagement
import com.codeiy.im.model.SubscriptionUsage
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** 可控延时/计数的假后端，只实现被测方法。 */
private class FakeRadarApi : RadarApi {
    var delayMillis = 0L
    var opportunityCalls = 0
    var templateCalls = 0
    var pages: (status: String?) -> List<Opportunity> = { emptyList() }

    override suspend fun opportunities(
        status: String?,
        platform: String?,
        limit: Int,
        offset: Int,
    ): List<Opportunity> {
        opportunityCalls++
        delay(delayMillis)
        return pages(status)
    }

    override suspend fun templates(): List<ReplyTemplate> {
        templateCalls++
        delay(delayMillis)
        return emptyList()
    }

    override suspend fun passwordLogin(body: PasswordLoginRequest): AuthToken = error("unused")
    override suspend fun googleNativeLogin(body: NativeLoginRequest): AuthToken = error("unused")
    override suspend fun me(): AuthUser = error("unused")
    override suspend fun opportunity(id: String): Opportunity = error("unused")
    override suspend fun messages(opportunityId: String): List<ChatMessage> = error("unused")
    override suspend fun manualReply(id: String, body: ManualReplyRequest): Opportunity = error("unused")
    override suspend fun aiDraft(id: String): AIDraft = error("unused")
    override suspend fun updateStatus(id: String, body: OpportunityStatusUpdate): Opportunity = error("unused")
    override suspend fun claim(id: String, operatorId: String): Opportunity = error("unused")
    override suspend fun subscription(): SubscriptionUsage = error("unused")
    override suspend fun subscriptionCatalog(): List<SubscriptionCatalogPlan> = error("unused")
    override suspend fun syncSubscription(): SubscriptionUsage = error("unused")
    override suspend fun subscriptionManagement(client: String): SubscriptionManagement = error("unused")
}

/** 收件箱竞态回归：筛选切换取消在途请求，旧响应不得覆盖新筛选结果。 */
@OptIn(ExperimentalCoroutinesApi::class)
class InboxViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun refreshCancelsInFlightRequestAndKeepsNewFilterResult() = runTest(dispatcher) {
        val api = FakeRadarApi()
        api.delayMillis = 1000
        api.pages = { status ->
            if (status == "pending") listOf(Opportunity(id = "pending-1")) else listOf(Opportunity(id = "all-1"))
        }
        val model = InboxViewModel(api)

        model.refresh() // 「全部」慢请求在途
        advanceTimeBy(100)
        model.setFilters(FrontendOpportunityStatus.PENDING, null)
        model.refresh() // 新筛选，应取消旧请求
        advanceUntilIdle()

        assertEquals(listOf("pending-1"), model.state.value.items.map { it.id })
        assertEquals(2, api.opportunityCalls)
    }

    @Test
    fun staleResponseDoesNotOverwriteAfterFilterChange() = runTest(dispatcher) {
        val api = FakeRadarApi()
        api.delayMillis = 1000
        api.pages = { listOf(Opportunity(id = "stale")) }
        val model = InboxViewModel(api)

        model.refresh()
        advanceTimeBy(100)
        // setFilters 与界面下一次 refresh 之间的窗口：旧响应返回也不得写入
        model.setFilters(FrontendOpportunityStatus.PENDING, null)
        advanceUntilIdle()

        assertTrue(model.state.value.items.isEmpty())
    }

    @Test
    fun pollingRefreshDoesNotStackRequests() = runTest(dispatcher) {
        val api = FakeRadarApi()
        api.delayMillis = 1000
        val model = InboxViewModel(api)

        model.refresh().join() // 界面轮询等待本轮完成后才发起下一轮
        model.refresh().join()

        assertEquals(2, api.opportunityCalls)
        assertTrue(!model.state.value.isLoading)
    }
}

/** 详情页模板加载：只加载一次，重组/重复打开不重复请求。 */
@OptIn(ExperimentalCoroutinesApi::class)
class OpportunityDetailModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun templatesLoadOnlyOnce() = runTest(dispatcher) {
        val api = FakeRadarApi()
        val model = OpportunityDetailModel(api, "opp-1") { "operator" }

        model.loadTemplatesIfNeeded()
        model.loadTemplatesIfNeeded() // 加载中重入：不追加请求
        advanceUntilIdle()
        model.loadTemplatesIfNeeded() // 已加载：不再请求
        advanceUntilIdle()

        assertEquals(1, api.templateCalls)
        assertTrue(model.state.value.templatesLoaded)
    }
}
