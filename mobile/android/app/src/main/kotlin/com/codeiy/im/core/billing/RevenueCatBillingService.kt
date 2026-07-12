package com.codeiy.im.core.billing

import android.app.Activity
import android.content.Context
import com.codeiy.im.BuildConfig
import com.codeiy.im.model.BillingInterval
import com.codeiy.im.model.PlanCode
import com.revenuecat.purchases.Package
import com.revenuecat.purchases.PurchaseParams
import com.revenuecat.purchases.Purchases
import com.revenuecat.purchases.PurchasesConfiguration
import com.revenuecat.purchases.PurchasesTransactionException
import com.revenuecat.purchases.awaitLogIn
import com.revenuecat.purchases.awaitLogOut
import com.revenuecat.purchases.awaitOfferings
import com.revenuecat.purchases.awaitPurchase
import com.revenuecat.purchases.awaitRestore

class RevenueCatBillingService(context: Context) : BillingService {
    private val applicationContext = context.applicationContext
    private val apiKey = BuildConfig.REVENUECAT_ANDROID_PUBLIC_API_KEY.trim()
    private var currentUserId: String? = null
    private val packages = mutableMapOf<String, Package>()

    override val isConfigured: Boolean get() = apiKey.isNotEmpty()

    override suspend fun identify(userId: String) {
        require(userId.isNotBlank()) { "登录完成后才能购买套餐" }
        if (!isConfigured) throw BillingUnavailableException("支付尚未配置")
        if (currentUserId == userId) return
        if (Purchases.isConfigured) {
            Purchases.sharedInstance.awaitLogIn(userId)
        } else {
            Purchases.configure(
                PurchasesConfiguration.Builder(applicationContext, apiKey)
                    .appUserID(userId)
                    .build(),
            )
        }
        currentUserId = userId
        packages.clear()
    }

    override suspend fun clearIdentity() {
        packages.clear()
        currentUserId = null
        if (Purchases.isConfigured && !Purchases.sharedInstance.isAnonymous) {
            runCatching { Purchases.sharedInstance.awaitLogOut() }
        }
    }

    override suspend fun fetchPackages(): List<BillingPackageOption> {
        if (currentUserId == null) throw BillingUnavailableException("登录完成后才能购买套餐")
        val offering = Purchases.sharedInstance.awaitOfferings().all["default"]
            ?: throw BillingUnavailableException("当前没有可购买的套餐")
        return offering.availablePackages.mapNotNull { item ->
            val identity = packageIdentity(item.identifier) ?: return@mapNotNull null
            packages[item.identifier] = item
            BillingPackageOption(item.identifier, identity.first, identity.second, item.product.price.formatted)
        }
    }

    override suspend fun purchase(activity: Activity, packageId: String): BillingPurchaseResult {
        if (currentUserId == null) throw BillingUnavailableException("登录完成后才能购买套餐")
        val selected = packages[packageId] ?: throw BillingUnavailableException("所选套餐暂不可购买")
        return try {
            Purchases.sharedInstance.awaitPurchase(PurchaseParams.Builder(activity, selected).build())
            BillingPurchaseResult.Purchased
        } catch (error: PurchasesTransactionException) {
            if (error.userCancelled) BillingPurchaseResult.Cancelled else throw error
        }
    }

    override suspend fun restorePurchases() {
        if (currentUserId == null) throw BillingUnavailableException("登录完成后才能恢复购买")
        Purchases.sharedInstance.awaitRestore()
    }

    private fun packageIdentity(identifier: String): Pair<PlanCode, BillingInterval>? = when (identifier) {
        "plus_monthly" -> PlanCode.PLUS to BillingInterval.MONTHLY
        "plus_annual" -> PlanCode.PLUS to BillingInterval.ANNUAL
        "pro_monthly" -> PlanCode.PRO to BillingInterval.MONTHLY
        "pro_annual" -> PlanCode.PRO to BillingInterval.ANNUAL
        "max_monthly" -> PlanCode.MAX to BillingInterval.MONTHLY
        "max_annual" -> PlanCode.MAX to BillingInterval.ANNUAL
        else -> null
    }
}
