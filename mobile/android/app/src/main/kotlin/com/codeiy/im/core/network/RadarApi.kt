package com.codeiy.im.core.network

import com.codeiy.im.BuildConfig
import com.codeiy.im.model.AIDraft
import com.codeiy.im.model.AuthToken
import com.codeiy.im.model.AuthUser
import com.codeiy.im.model.ChatMessage
import com.codeiy.im.model.DashboardResponse
import com.codeiy.im.model.DetectionSettings
import com.codeiy.im.model.DetectionSettingsUpdate
import com.codeiy.im.model.JobFeedbackRequest
import com.codeiy.im.model.JobFeedbackResponse
import com.codeiy.im.model.JobOpportunityDetail
import com.codeiy.im.model.JobProfileParseRequest
import com.codeiy.im.model.JobSearchProfile
import com.codeiy.im.model.JobSearchProfilePreview
import com.codeiy.im.model.JobSearchProfileWrite
import com.codeiy.im.model.JobsPage
import com.codeiy.im.model.ManualReplyRequest
import com.codeiy.im.model.NativeLoginRequest
import com.codeiy.im.model.NotificationSettings
import com.codeiy.im.model.NotificationSettingsUpdate
import com.codeiy.im.model.Opportunity
import com.codeiy.im.model.OpportunityStatusUpdate
import com.codeiy.im.model.PasswordLoginRequest
import com.codeiy.im.model.PasswordActionResponse
import com.codeiy.im.model.PasswordChangeRequest
import com.codeiy.im.model.PasswordResetConfirmRequest
import com.codeiy.im.model.PasswordResetRequest
import com.codeiy.im.model.ReplyTemplate
import com.codeiy.im.model.SettingsBundle
import com.codeiy.im.model.SubscriptionCatalogPlan
import com.codeiy.im.model.SubscriptionManagement
import com.codeiy.im.model.SubscriptionUsage
import com.codeiy.im.model.TelegramConnectionDTO
import com.codeiy.im.model.TelegramConnectionEnabledUpdate
import com.codeiy.im.model.TelegramConnectionHealth
import com.codeiy.im.model.WorkSchedule
import com.codeiy.im.model.WorkScheduleUpdate
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

/** 路径/参数以 backend/app/api/v1/routes/ 为准，与 iOS Endpoints.swift 对齐。 */
interface RadarApi {
    @POST("auth/password/login")
    suspend fun passwordLogin(@Body body: PasswordLoginRequest): AuthToken

    @POST("auth/password/reset/request")
    suspend fun requestPasswordReset(@Body body: PasswordResetRequest): PasswordActionResponse

    @POST("auth/password/reset/confirm")
    suspend fun confirmPasswordReset(@Body body: PasswordResetConfirmRequest): PasswordActionResponse

    @POST("auth/password/change")
    suspend fun changePassword(@Body body: PasswordChangeRequest): PasswordActionResponse

    @POST("auth/oauth/google/native")
    suspend fun googleNativeLogin(@Body body: NativeLoginRequest): AuthToken

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

    // 商机看板：数组参数用重复 query（Retrofit 对 List 自动展开为重复 key）。
    @GET("opportunities/dashboard")
    suspend fun dashboard(
        @Query("status") status: String? = null,
        @Query("platform") platform: String? = null,
        @Query("source_type") sourceType: String? = null,
        @Query("created_from") createdFrom: String? = null,
        @Query("created_to") createdTo: String? = null,
        @Query("trust_levels") trustLevels: List<String>? = null,
        @Query("sop_stages") sopStages: List<String>? = null,
        @Query("keywords") keywords: List<String>? = null,
        @Query("sort") sort: String = "newest",
        @Query("limit") limit: Int = 20,
        @Query("offset") offset: Int = 0,
    ): DashboardResponse

    // 用户级设置
    @GET("settings/me")
    suspend fun settings(): SettingsBundle

    @PATCH("settings/detection")
    suspend fun updateDetection(@Body body: DetectionSettingsUpdate): DetectionSettings

    @PATCH("settings/work-schedule")
    suspend fun updateWorkSchedule(@Body body: WorkScheduleUpdate): WorkSchedule

    @PATCH("settings/notifications")
    suspend fun updateNotifications(@Body body: NotificationSettingsUpdate): NotificationSettings

    // Telegram 连接（真实读取 + 启用/停用）
    @GET("integrations/telegram/health")
    suspend fun telegramHealth(): TelegramConnectionHealth

    @GET("integrations/telegram/connections")
    suspend fun telegramConnections(): List<TelegramConnectionDTO>

    @PATCH("integrations/telegram/connections/{id}")
    suspend fun setTelegramConnectionEnabled(
        @Path("id") id: String,
        @Body body: TelegramConnectionEnabledUpdate,
    ): TelegramConnectionDTO

    // 工作机会发现：服务端是筛选、匹配分和 owner 隔离的最终权威。
    @GET("jobs")
    suspend fun jobs(
        @Query("profile_id") profileId: String? = null,
        @Query("query") query: String? = null,
        @Query("source") source: String? = null,
        @Query("work_mode") workMode: String? = null,
        @Query("employment_type") employmentType: String? = null,
        @Query("seniority") seniority: String? = null,
        @Query("minimum_match_score") minimumMatchScore: Int? = null,
        @Query("age_requirement_present") ageRequirementPresent: Boolean? = null,
        @Query("exclude_expired") excludeExpired: Boolean = true,
        @Query("sort") sort: String = "match",
        @Query("limit") limit: Int = 20,
        @Query("offset") offset: Int = 0,
    ): JobsPage

    @GET("jobs/{id}")
    suspend fun job(@Path("id") id: String, @Query("profile_id") profileId: String? = null): JobOpportunityDetail

    @POST("jobs/{id}/feedback")
    suspend fun saveJobFeedback(@Path("id") id: String, @Body body: JobFeedbackRequest): JobFeedbackResponse

    @GET("job-search-profiles")
    suspend fun jobSearchProfiles(): List<JobSearchProfile>

    @POST("job-search-profiles")
    suspend fun createJobSearchProfile(@Body body: JobSearchProfileWrite): JobSearchProfile

    @PATCH("job-search-profiles/{id}")
    suspend fun updateJobSearchProfile(@Path("id") id: String, @Body body: JobSearchProfileWrite): JobSearchProfile

    @DELETE("job-search-profiles/{id}")
    suspend fun deleteJobSearchProfile(@Path("id") id: String)

    @POST("job-search-profiles/parse")
    suspend fun parseJobSearchProfile(@Body body: JobProfileParseRequest): JobSearchProfilePreview
}

val ApiBaseUrl: String get() = BuildConfig.API_BASE_URL
