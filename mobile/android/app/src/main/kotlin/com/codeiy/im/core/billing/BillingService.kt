package com.codeiy.im.core.billing

import android.app.Activity
import com.codeiy.im.model.BillingInterval
import com.codeiy.im.model.PlanCode

data class BillingPackageOption(
    val id: String,
    val planCode: PlanCode,
    val interval: BillingInterval,
    val localizedPrice: String,
)

sealed interface BillingPurchaseResult {
    data object Purchased : BillingPurchaseResult
    data object Cancelled : BillingPurchaseResult
}

class BillingUnavailableException(message: String) : IllegalStateException(message)

interface BillingService {
    val isConfigured: Boolean
    suspend fun identify(userId: String)
    suspend fun clearIdentity()
    suspend fun fetchPackages(): List<BillingPackageOption>
    suspend fun purchase(activity: Activity, packageId: String): BillingPurchaseResult
    suspend fun restorePurchases()
}
