package com.codeiy.im

import android.app.Activity
import com.codeiy.im.core.billing.BillingPackageOption
import com.codeiy.im.core.billing.BillingPurchaseResult
import com.codeiy.im.core.billing.BillingService
import com.codeiy.im.feature.subscription.SubscriptionBackend
import com.codeiy.im.feature.subscription.SubscriptionViewModel
import com.codeiy.im.model.BillingInterval
import com.codeiy.im.model.PlanCode
import com.codeiy.im.model.PlanEntitlements
import com.codeiy.im.model.RadarJson
import com.codeiy.im.model.SubscriptionCatalogPlan
import com.codeiy.im.model.SubscriptionManagement
import com.codeiy.im.model.SubscriptionStatus
import com.codeiy.im.model.SubscriptionUsage
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class SubscriptionTests {
    private val dispatcher = StandardTestDispatcher()

    @Before fun setUp() = Dispatchers.setMain(dispatcher)
    @After fun tearDown() = Dispatchers.resetMain()

    @Test fun annualSubscriptionKeepsMonthlyUsagePeriod() {
        val value = RadarJson.decodeFromString<SubscriptionUsage>("""
          {"planCode":"pro","subscriptionStatus":"active","periodStart":"2026-07-01T00:00:00Z",
          "periodEnd":"2026-08-01T00:00:00Z","entitlements":{"planCode":"pro","combinedGroupLimit":20,
          "piAgentAnalysisMonthlyLimit":1000},"billingInterval":"annual",
          "billingPeriodStart":"2026-05-10T00:00:00Z","billingPeriodEnd":"2027-05-10T00:00:00Z",
          "usagePeriodStart":"2026-07-01T00:00:00Z","usagePeriodEnd":"2026-08-01T00:00:00Z"}
        """.trimIndent())
        assertEquals(BillingInterval.ANNUAL, value.billingInterval)
        assertNotEquals(value.billingPeriodEnd, value.usagePeriodEnd)
    }

    @Test fun purchaseSuccessSynchronizesBackend() = runTest {
        val backend = FakeBackend()
        val billing = FakeBilling(BillingPurchaseResult.Purchased)
        val model = SubscriptionViewModel(USER_ID, backend, billing)
        model.load().join()
        model.purchase(mockk(relaxed = true), model.packageFor(PlanCode.PLUS)!!).join()
        assertEquals(1, backend.syncCount)
        assertEquals(PlanCode.PLUS, model.state.value.usage?.planCode)
        assertEquals(USER_ID, billing.identifiedUser)
    }

    @Test fun purchaseCancellationDoesNotSynchronizeBackend() = runTest {
        val backend = FakeBackend()
        val model = SubscriptionViewModel(USER_ID, backend, FakeBilling(BillingPurchaseResult.Cancelled))
        model.load().join()
        model.purchase(mockk(relaxed = true), model.packageFor(PlanCode.PLUS)!!).join()
        assertEquals(0, backend.syncCount)
        assertEquals("已取消购买，未产生套餐变更。", model.state.value.message)
    }

    @Test fun restoreIdentifiesUserAndSynchronizesBackend() = runTest {
        val backend = FakeBackend()
        val billing = FakeBilling(BillingPurchaseResult.Purchased)
        val model = SubscriptionViewModel(USER_ID, backend, billing)
        model.restore().join()
        assertEquals(USER_ID, billing.identifiedUser)
        assertEquals(1, billing.restoreCount)
        assertEquals(1, backend.syncCount)
    }

    @Test fun unconfiguredBillingCannotPurchaseAnonymously() = runTest {
        val backend = FakeBackend()
        val billing = FakeBilling(BillingPurchaseResult.Purchased, isConfigured = false)
        val model = SubscriptionViewModel(USER_ID, backend, billing)
        model.load().join()
        model.purchase(mockk(relaxed = true), option).join()
        assertEquals(0, billing.purchaseCount)
        assertEquals(0, backend.syncCount)
    }

    companion object {
        const val USER_ID = "703c41d7-9abd-42ae-9778-b543b489fc51"
        val option = BillingPackageOption("plus_monthly", PlanCode.PLUS, BillingInterval.MONTHLY, "$1")
    }
}

private class FakeBilling(private val result: BillingPurchaseResult, override val isConfigured: Boolean = true) : BillingService {
    var identifiedUser: String? = null
    var restoreCount = 0
    var purchaseCount = 0
    override suspend fun identify(userId: String) { identifiedUser = userId }
    override suspend fun clearIdentity() { identifiedUser = null }
    override suspend fun fetchPackages() = listOf(SubscriptionTests.option)
    override suspend fun purchase(activity: Activity, packageId: String): BillingPurchaseResult { purchaseCount++; return result }
    override suspend fun restorePurchases() { restoreCount++ }
}

private class FakeBackend : SubscriptionBackend {
    var syncCount = 0
    override suspend fun usage() = usage(PlanCode.FREE)
    override suspend fun catalog() = listOf(SubscriptionCatalogPlan(PlanCode.PLUS, "Plus", 1, entitlements(PlanCode.PLUS), listOf(BillingInterval.MONTHLY, BillingInterval.ANNUAL), listOf("plus_monthly", "plus_annual")))
    override suspend fun sync(): SubscriptionUsage { syncCount++; return usage(PlanCode.PLUS) }
    override suspend fun management() = SubscriptionManagement(instruction = "", canOpenInCurrentClient = false)
    private fun entitlements(plan: PlanCode) = PlanEntitlements(plan, 1, 1, 2, 10)
    private fun usage(plan: PlanCode) = SubscriptionUsage(plan, if (plan == PlanCode.FREE) SubscriptionStatus.INACTIVE else SubscriptionStatus.ACTIVE,
        entitlements = entitlements(plan), usagePeriodStart = "2026-07-01T00:00:00Z", usagePeriodEnd = "2026-08-01T00:00:00Z")
}
