package com.codeiy.im.core.network

import com.codeiy.im.BuildConfig
import com.codeiy.im.model.AIDraft
import com.codeiy.im.model.AuthToken
import com.codeiy.im.model.AuthUser
import com.codeiy.im.model.ChatMessage
import com.codeiy.im.model.ManualReplyRequest
import com.codeiy.im.model.Opportunity
import com.codeiy.im.model.OpportunityStatusUpdate
import com.codeiy.im.model.PasswordLoginRequest
import com.codeiy.im.model.ReplyTemplate
import com.codeiy.im.model.SubscriptionCatalogPlan
import com.codeiy.im.model.SubscriptionManagement
import com.codeiy.im.model.SubscriptionUsage
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

/** 路径/参数以 backend/app/api/v1/routes/ 为准，与 iOS Endpoints.swift 对齐。 */
interface RadarApi {
    @POST("auth/password/login")
    suspend fun passwordLogin(@Body body: PasswordLoginRequest): AuthToken

    @GET("auth/me")
    suspend fun me(): AuthUser

    @GET("opportunities")
    suspend fun opportunities(
        @Query("status") status: String? = null,
        @Query("platform") platform: String? = null,
        @Query("limit") limit: Int = 50,
        @Query("offset") offset: Int = 0,
    ): List<Opportunity>

    @GET("opportunities/{id}")
    suspend fun opportunity(@Path("id") id: String): Opportunity

    @GET("messages")
    suspend fun messages(@Query("opportunity_id") opportunityId: String): List<ChatMessage>

    @POST("opportunities/{id}/manual-reply")
    suspend fun manualReply(@Path("id") id: String, @Body body: ManualReplyRequest): Opportunity

    @POST("opportunities/{id}/ai-draft")
    suspend fun aiDraft(@Path("id") id: String): AIDraft

    @PATCH("opportunities/{id}/status")
    suspend fun updateStatus(@Path("id") id: String, @Body body: OpportunityStatusUpdate): Opportunity

    @POST("opportunities/{id}/claim")
    suspend fun claim(@Path("id") id: String, @Query("operator_id") operatorId: String): Opportunity

    @GET("templates")
    suspend fun templates(): List<ReplyTemplate>

    @GET("subscriptions/me")
    suspend fun subscription(): SubscriptionUsage

    @GET("subscriptions/catalog")
    suspend fun subscriptionCatalog(): List<SubscriptionCatalogPlan>

    @POST("subscriptions/sync")
    suspend fun syncSubscription(): SubscriptionUsage

    @GET("subscriptions/management")
    suspend fun subscriptionManagement(@Query("client") client: String = "android"): SubscriptionManagement
}

val ApiBaseUrl: String get() = BuildConfig.API_BASE_URL
