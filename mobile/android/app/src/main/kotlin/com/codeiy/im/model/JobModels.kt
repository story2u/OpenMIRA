package com.codeiy.im.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

@Serializable
enum class JobWorkMode(val label: String) {
    @SerialName("remote") REMOTE("远程"),
    @SerialName("hybrid") HYBRID("混合"),
    @SerialName("on_site") ON_SITE("现场"),
    @SerialName("flexible") FLEXIBLE("灵活"),
    UNKNOWN("未说明"),
}

@Serializable
enum class JobEmploymentType(val label: String) {
    @SerialName("full_time") FULL_TIME("全职"),
    @SerialName("part_time") PART_TIME("兼职"),
    @SerialName("contract") CONTRACT("合同"),
    @SerialName("internship") INTERNSHIP("实习"),
    @SerialName("freelance") FREELANCE("自由职业"),
    @SerialName("temporary") TEMPORARY("临时"),
    UNKNOWN("未说明"),
}

@Serializable
enum class JobSeniority(val label: String) {
    @SerialName("intern") INTERN("实习"),
    @SerialName("junior") JUNIOR("初级"),
    @SerialName("mid") MID("中级"),
    @SerialName("senior") SENIOR("高级"),
    @SerialName("lead") LEAD("负责人"),
    @SerialName("manager") MANAGER("经理"),
    @SerialName("director") DIRECTOR("总监"),
    @SerialName("executive") EXECUTIVE("高管"),
    UNKNOWN("未说明"),
}

@Serializable
enum class SalaryPeriod {
    @SerialName("hourly") HOURLY,
    @SerialName("daily") DAILY,
    @SerialName("monthly") MONTHLY,
    @SerialName("annual") ANNUAL,
    @SerialName("project") PROJECT,
    UNKNOWN,
}

@Serializable
enum class JobEligibility {
    @SerialName("eligible") ELIGIBLE,
    @SerialName("not_eligible") NOT_ELIGIBLE,
    UNKNOWN,
}

@Serializable
enum class JobFeedbackType(val label: String) {
    @SerialName("relevant") RELEVANT("适合我"),
    @SerialName("not_relevant") NOT_RELEVANT("不适合"),
    @SerialName("not_a_job") NOT_A_JOB("不是招聘"),
    @SerialName("duplicate") DUPLICATE("重复"),
    @SerialName("expired") EXPIRED("已过期"),
    @SerialName("scam") SCAM("疑似诈骗"),
    @SerialName("wrong_extraction") WRONG_EXTRACTION("提取错误"),
    UNKNOWN("其他"),
}

@Serializable
data class JobMatch(
    val eligibility: JobEligibility = JobEligibility.UNKNOWN,
    val matchScore: Int = 0,
    val matchedReasons: List<String> = emptyList(),
    val mismatchReasons: List<String> = emptyList(),
    val unknownConstraints: List<String> = emptyList(),
    val scoreBreakdown: Map<String, Int> = emptyMap(),
)

@Serializable
data class JobSource(
    val id: String,
    val channel: IMChannel = IMChannel.UNKNOWN,
    val chatName: String? = null,
    val authorName: String? = null,
    val postedAt: String = "",
    val sourceMessageUrl: String? = null,
    val reliabilityScore: Double = 0.0,
)

@Serializable
data class JobOpportunity(
    val opportunityId: String,
    val jobTitle: String = "",
    val companyName: String? = null,
    val sourceChannel: IMChannel = IMChannel.UNKNOWN,
    val sourceChatName: String? = null,
    val postedAt: String = "",
    val locationText: String? = null,
    val countryCode: String? = null,
    val city: String? = null,
    val workMode: JobWorkMode = JobWorkMode.UNKNOWN,
    val employmentType: JobEmploymentType = JobEmploymentType.UNKNOWN,
    val seniority: JobSeniority = JobSeniority.UNKNOWN,
    val salaryRaw: String? = null,
    val salaryMin: Double? = null,
    val salaryMax: Double? = null,
    val salaryCurrency: String? = null,
    val salaryPeriod: SalaryPeriod = SalaryPeriod.UNKNOWN,
    val requiredSkills: List<String> = emptyList(),
    val degreeLevel: String? = null,
    val englishLevel: String? = null,
    val visaSponsorship: Boolean? = null,
    val applicationDeadline: String? = null,
    val sourceReliabilityScore: Double = 0.0,
    val extractionConfidence: Double = 0.0,
    val sourceCount: Int = 1,
    val conflictingSourceData: Boolean = false,
    val complianceFlags: List<String> = emptyList(),
    val isExpired: Boolean = false,
    val match: JobMatch? = null,
)

@Serializable
data class JobOpportunityDetail(
    val opportunityId: String,
    val jobTitle: String = "",
    val companyName: String? = null,
    val sourceChannel: IMChannel = IMChannel.UNKNOWN,
    val sourceChatName: String? = null,
    val postedAt: String = "",
    val locationText: String? = null,
    val countryCode: String? = null,
    val city: String? = null,
    val workMode: JobWorkMode = JobWorkMode.UNKNOWN,
    val employmentType: JobEmploymentType = JobEmploymentType.UNKNOWN,
    val seniority: JobSeniority = JobSeniority.UNKNOWN,
    val salaryRaw: String? = null,
    val salaryMin: Double? = null,
    val salaryMax: Double? = null,
    val salaryCurrency: String? = null,
    val salaryPeriod: SalaryPeriod = SalaryPeriod.UNKNOWN,
    val requiredSkills: List<String> = emptyList(),
    val degreeLevel: String? = null,
    val englishLevel: String? = null,
    val visaSponsorship: Boolean? = null,
    val applicationDeadline: String? = null,
    val sourceReliabilityScore: Double = 0.0,
    val extractionConfidence: Double = 0.0,
    val sourceCount: Int = 1,
    val conflictingSourceData: Boolean = false,
    val complianceFlags: List<String> = emptyList(),
    val isExpired: Boolean = false,
    val match: JobMatch? = null,
    val sourceMessageUrl: String? = null,
    val sourceAuthorName: String? = null,
    val department: String? = null,
    val companyIndustry: String? = null,
    val companyStage: String? = null,
    val timezone: String? = null,
    val salaryNegotiable: Boolean? = null,
    val equityMentioned: Boolean? = null,
    val requirementsSummary: String? = null,
    val preferredSkills: List<String> = emptyList(),
    val minimumYearsExperience: Double? = null,
    val maximumYearsExperience: Double? = null,
    val degreeRequired: Boolean? = null,
    val degreeField: String? = null,
    val otherLanguageRequirements: List<String> = emptyList(),
    val workAuthorizationText: String? = null,
    val relocationSupport: Boolean? = null,
    val ageRequirementText: String? = null,
    val ageRequirementPresent: Boolean = false,
    val applicationUrl: String? = null,
    val contactMethods: List<Map<String, String>> = emptyList(),
    val missingFields: List<String> = emptyList(),
    val fieldEvidence: Map<String, String> = emptyMap(),
    val rawExcerpt: String = "",
    val expiredReason: String? = null,
    val sources: List<JobSource> = emptyList(),
)

@Serializable
data class JobSearchProfileWrite(
    val name: String = "",
    val isDefault: Boolean = false,
    val enabled: Boolean = true,
    val targetRoles: List<String> = emptyList(),
    val excludedRoles: List<String> = emptyList(),
    val targetIndustries: List<String> = emptyList(),
    val preferredSeniority: List<JobSeniority> = emptyList(),
    val candidateSkills: List<String> = emptyList(),
    val yearsExperience: Double? = null,
    val educationLevel: String? = null,
    val englishLevel: String? = null,
    val otherLanguages: List<String> = emptyList(),
    val preferredCountries: List<String> = emptyList(),
    val preferredCities: List<String> = emptyList(),
    val preferredTimezones: List<String> = emptyList(),
    val workModes: List<JobWorkMode> = emptyList(),
    val employmentTypes: List<JobEmploymentType> = emptyList(),
    val minimumSalary: Double? = null,
    val salaryCurrency: String? = null,
    val salaryPeriod: SalaryPeriod? = null,
    val visaSponsorshipRequired: Boolean? = null,
    val relocationAcceptable: Boolean? = null,
    val requiredKeywords: List<String> = emptyList(),
    val preferredKeywords: List<String> = emptyList(),
    val excludedKeywords: List<String> = emptyList(),
    val requireSalaryDisclosed: Boolean = false,
    val minimumMatchScore: Int = 0,
    val notificationEnabled: Boolean = false,
)

@Serializable
data class JobSearchProfile(
    val id: String,
    val name: String = "",
    val isDefault: Boolean = false,
    val enabled: Boolean = true,
    val targetRoles: List<String> = emptyList(),
    val excludedRoles: List<String> = emptyList(),
    val targetIndustries: List<String> = emptyList(),
    val preferredSeniority: List<JobSeniority> = emptyList(),
    val candidateSkills: List<String> = emptyList(),
    val yearsExperience: Double? = null,
    val educationLevel: String? = null,
    val englishLevel: String? = null,
    val otherLanguages: List<String> = emptyList(),
    val preferredCountries: List<String> = emptyList(),
    val preferredCities: List<String> = emptyList(),
    val preferredTimezones: List<String> = emptyList(),
    val workModes: List<JobWorkMode> = emptyList(),
    val employmentTypes: List<JobEmploymentType> = emptyList(),
    val minimumSalary: Double? = null,
    val salaryCurrency: String? = null,
    val salaryPeriod: SalaryPeriod? = null,
    val visaSponsorshipRequired: Boolean? = null,
    val relocationAcceptable: Boolean? = null,
    val requiredKeywords: List<String> = emptyList(),
    val preferredKeywords: List<String> = emptyList(),
    val excludedKeywords: List<String> = emptyList(),
    val requireSalaryDisclosed: Boolean = false,
    val minimumMatchScore: Int = 0,
    val notificationEnabled: Boolean = false,
    val createdAt: String = "",
    val updatedAt: String = "",
) {
    fun toWrite() = JobSearchProfileWrite(
        name, isDefault, enabled, targetRoles, excludedRoles, targetIndustries,
        preferredSeniority, candidateSkills, yearsExperience, educationLevel, englishLevel,
        otherLanguages, preferredCountries, preferredCities, preferredTimezones, workModes,
        employmentTypes, minimumSalary, salaryCurrency, salaryPeriod, visaSponsorshipRequired,
        relocationAcceptable, requiredKeywords, preferredKeywords, excludedKeywords,
        requireSalaryDisclosed, minimumMatchScore, notificationEnabled,
    )
}

@Serializable
data class JobSearchProfilePreview(
    val name: String = "",
    val isDefault: Boolean = false,
    val enabled: Boolean = true,
    val targetRoles: List<String> = emptyList(),
    val excludedRoles: List<String> = emptyList(),
    val targetIndustries: List<String> = emptyList(),
    val preferredSeniority: List<JobSeniority> = emptyList(),
    val candidateSkills: List<String> = emptyList(),
    val yearsExperience: Double? = null,
    val educationLevel: String? = null,
    val englishLevel: String? = null,
    val otherLanguages: List<String> = emptyList(),
    val preferredCountries: List<String> = emptyList(),
    val preferredCities: List<String> = emptyList(),
    val preferredTimezones: List<String> = emptyList(),
    val workModes: List<JobWorkMode> = emptyList(),
    val employmentTypes: List<JobEmploymentType> = emptyList(),
    val minimumSalary: Double? = null,
    val salaryCurrency: String? = null,
    val salaryPeriod: SalaryPeriod? = null,
    val visaSponsorshipRequired: Boolean? = null,
    val relocationAcceptable: Boolean? = null,
    val requiredKeywords: List<String> = emptyList(),
    val preferredKeywords: List<String> = emptyList(),
    val excludedKeywords: List<String> = emptyList(),
    val requireSalaryDisclosed: Boolean = false,
    val minimumMatchScore: Int = 0,
    val notificationEnabled: Boolean = false,
    val requiresConfirmation: Boolean = true,
) {
    fun toWrite() = JobSearchProfileWrite(
        name, isDefault, enabled, targetRoles, excludedRoles, targetIndustries,
        preferredSeniority, candidateSkills, yearsExperience, educationLevel, englishLevel,
        otherLanguages, preferredCountries, preferredCities, preferredTimezones, workModes,
        employmentTypes, minimumSalary, salaryCurrency, salaryPeriod, visaSponsorshipRequired,
        relocationAcceptable, requiredKeywords, preferredKeywords, excludedKeywords,
        requireSalaryDisclosed, minimumMatchScore, notificationEnabled,
    )
}

@Serializable
data class JobsPage(
    val items: List<JobOpportunity> = emptyList(),
    val total: Int = 0,
    val limit: Int = 20,
    val offset: Int = 0,
    val filterSummary: Map<String, JsonElement> = emptyMap(),
    val profile: JobSearchProfile? = null,
)

@Serializable
data class JobProfileParseRequest(val text: String)

@Serializable
data class JobFeedbackRequest(val feedbackType: JobFeedbackType, val note: String? = null)

@Serializable
data class JobFeedbackResponse(
    val id: String,
    val feedbackType: JobFeedbackType,
    val note: String? = null,
    val updatedAt: String = "",
)

