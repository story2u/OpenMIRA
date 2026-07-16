import Foundation

enum JobWorkMode: String, TolerantEnum, CaseIterable {
    case remote, hybrid, onSite = "on_site", flexible, unknown
    var label: String { switch self { case .remote: "远程"; case .hybrid: "混合"; case .onSite: "现场"; case .flexible: "灵活"; case .unknown: "未说明" } }
}

enum JobEmploymentType: String, TolerantEnum, CaseIterable {
    case fullTime = "full_time", partTime = "part_time", contract, internship, freelance, temporary, unknown
    var label: String { switch self { case .fullTime: "全职"; case .partTime: "兼职"; case .contract: "合同"; case .internship: "实习"; case .freelance: "自由职业"; case .temporary: "临时"; case .unknown: "未说明" } }
}

enum JobSeniority: String, TolerantEnum, CaseIterable {
    case intern, junior, mid, senior, lead, manager, director, executive, unknown
    var label: String { switch self { case .intern: "实习"; case .junior: "初级"; case .mid: "中级"; case .senior: "高级"; case .lead: "负责人"; case .manager: "经理"; case .director: "总监"; case .executive: "高管"; case .unknown: "未说明" } }
}

enum SalaryPeriod: String, TolerantEnum, CaseIterable {
    case hourly, daily, monthly, annual, project, unknown
}

enum JobEligibility: String, TolerantEnum {
    case eligible, notEligible = "not_eligible", unknown
}

enum JobFeedbackType: String, TolerantEnum, CaseIterable {
    case relevant, notRelevant = "not_relevant", notAJob = "not_a_job", duplicate, expired, scam, wrongExtraction = "wrong_extraction", unknown
    var label: String { switch self { case .relevant: "适合我"; case .notRelevant: "不适合"; case .notAJob: "不是招聘"; case .duplicate: "重复"; case .expired: "已过期"; case .scam: "疑似诈骗"; case .wrongExtraction: "提取错误"; case .unknown: "其他" } }
}

struct JobMatch: Codable, Sendable, Hashable {
    var eligibility: JobEligibility
    var matchScore: Int
    var matchedReasons: [String]
    var mismatchReasons: [String]
    var unknownConstraints: [String]
    var scoreBreakdown: [String: Int]
}

struct JobSource: Codable, Sendable, Identifiable, Hashable {
    var id: UUID
    var channel: IMChannel
    var chatName: String?
    var authorName: String?
    var postedAt: Date
    var sourceMessageUrl: URL?
    var reliabilityScore: Double
}

struct JobOpportunity: Codable, Sendable, Identifiable, Hashable {
    var opportunityId: UUID
    var jobTitle: String
    var companyName: String?
    var sourceChannel: IMChannel
    var sourceChatName: String?
    var postedAt: Date
    var locationText: String?
    var countryCode: String?
    var city: String?
    var workMode: JobWorkMode
    var employmentType: JobEmploymentType
    var seniority: JobSeniority
    var salaryRaw: String?
    var salaryMin: Double?
    var salaryMax: Double?
    var salaryCurrency: String?
    var salaryPeriod: SalaryPeriod
    var requiredSkills: [String]
    var degreeLevel: String?
    var englishLevel: String?
    var visaSponsorship: Bool?
    var applicationDeadline: Date?
    var sourceReliabilityScore: Double
    var extractionConfidence: Double
    var sourceCount: Int
    var conflictingSourceData: Bool
    var complianceFlags: [String]
    var isExpired: Bool
    var match: JobMatch?
    var id: UUID { opportunityId }
}

struct JobContactMethod: Codable, Sendable, Hashable { var type: String; var value: String }

struct JobOpportunityDetail: Codable, Sendable, Identifiable, Hashable {
    var opportunityId: UUID
    var jobTitle: String
    var companyName: String?
    var sourceChannel: IMChannel
    var sourceChatName: String?
    var postedAt: Date
    var locationText: String?
    var countryCode: String?
    var city: String?
    var workMode: JobWorkMode
    var employmentType: JobEmploymentType
    var seniority: JobSeniority
    var salaryRaw: String?
    var salaryMin: Double?
    var salaryMax: Double?
    var salaryCurrency: String?
    var salaryPeriod: SalaryPeriod
    var requiredSkills: [String]
    var degreeLevel: String?
    var englishLevel: String?
    var visaSponsorship: Bool?
    var applicationDeadline: Date?
    var sourceReliabilityScore: Double
    var extractionConfidence: Double
    var sourceCount: Int
    var conflictingSourceData: Bool
    var complianceFlags: [String]
    var isExpired: Bool
    var match: JobMatch?
    var sourceMessageUrl: URL?
    var sourceAuthorName: String?
    var department: String?
    var companyIndustry: String?
    var companyStage: String?
    var timezone: String?
    var salaryNegotiable: Bool?
    var equityMentioned: Bool?
    var requirementsSummary: String?
    var preferredSkills: [String]
    var minimumYearsExperience: Double?
    var maximumYearsExperience: Double?
    var degreeRequired: Bool?
    var degreeField: String?
    var otherLanguageRequirements: [String]
    var workAuthorizationText: String?
    var relocationSupport: Bool?
    var ageRequirementText: String?
    var ageRequirementPresent: Bool
    var applicationUrl: URL?
    var contactMethods: [JobContactMethod]
    var missingFields: [String]
    var fieldEvidence: [String: String]
    var rawExcerpt: String
    var expiredReason: String?
    var sources: [JobSource]
    var id: UUID { opportunityId }
}

struct JobSearchProfile: Codable, Sendable, Identifiable, Hashable {
    var id: UUID
    var name: String
    var isDefault: Bool
    var enabled: Bool
    var targetRoles: [String]
    var excludedRoles: [String]
    var targetIndustries: [String]
    var preferredSeniority: [JobSeniority]
    var candidateSkills: [String]
    var yearsExperience: Double?
    var educationLevel: String?
    var englishLevel: String?
    var otherLanguages: [String]
    var preferredCountries: [String]
    var preferredCities: [String]
    var preferredTimezones: [String]
    var workModes: [JobWorkMode]
    var employmentTypes: [JobEmploymentType]
    var minimumSalary: Double?
    var salaryCurrency: String?
    var salaryPeriod: SalaryPeriod?
    var visaSponsorshipRequired: Bool?
    var relocationAcceptable: Bool?
    var requiredKeywords: [String]
    var preferredKeywords: [String]
    var excludedKeywords: [String]
    var requireSalaryDisclosed: Bool
    var minimumMatchScore: Int
    var notificationEnabled: Bool
    var createdAt: Date
    var updatedAt: Date
}

struct JobSearchProfileWrite: Codable, Sendable, Hashable {
    var name = ""
    var isDefault = false
    var enabled = true
    var targetRoles: [String] = []
    var excludedRoles: [String] = []
    var targetIndustries: [String] = []
    var preferredSeniority: [JobSeniority] = []
    var candidateSkills: [String] = []
    var yearsExperience: Double?
    var educationLevel: String?
    var englishLevel: String?
    var otherLanguages: [String] = []
    var preferredCountries: [String] = []
    var preferredCities: [String] = []
    var preferredTimezones: [String] = []
    var workModes: [JobWorkMode] = []
    var employmentTypes: [JobEmploymentType] = []
    var minimumSalary: Double?
    var salaryCurrency: String?
    var salaryPeriod: SalaryPeriod?
    var visaSponsorshipRequired: Bool?
    var relocationAcceptable: Bool?
    var requiredKeywords: [String] = []
    var preferredKeywords: [String] = []
    var excludedKeywords: [String] = []
    var requireSalaryDisclosed = false
    var minimumMatchScore = 0
    var notificationEnabled = false

    init() {}
    init(_ profile: JobSearchProfile) {
        name = profile.name; isDefault = profile.isDefault; enabled = profile.enabled
        targetRoles = profile.targetRoles; excludedRoles = profile.excludedRoles
        targetIndustries = profile.targetIndustries; preferredSeniority = profile.preferredSeniority
        candidateSkills = profile.candidateSkills; yearsExperience = profile.yearsExperience
        educationLevel = profile.educationLevel; englishLevel = profile.englishLevel
        otherLanguages = profile.otherLanguages; preferredCountries = profile.preferredCountries
        preferredCities = profile.preferredCities; preferredTimezones = profile.preferredTimezones
        workModes = profile.workModes; employmentTypes = profile.employmentTypes
        minimumSalary = profile.minimumSalary; salaryCurrency = profile.salaryCurrency
        salaryPeriod = profile.salaryPeriod; visaSponsorshipRequired = profile.visaSponsorshipRequired
        relocationAcceptable = profile.relocationAcceptable; requiredKeywords = profile.requiredKeywords
        preferredKeywords = profile.preferredKeywords; excludedKeywords = profile.excludedKeywords
        requireSalaryDisclosed = profile.requireSalaryDisclosed
        minimumMatchScore = profile.minimumMatchScore; notificationEnabled = profile.notificationEnabled
    }
}

struct JobSearchProfilePreview: Codable, Sendable {
    var name: String
    var isDefault: Bool
    var enabled: Bool
    var targetRoles: [String]
    var excludedRoles: [String]
    var targetIndustries: [String]
    var preferredSeniority: [JobSeniority]
    var candidateSkills: [String]
    var yearsExperience: Double?
    var educationLevel: String?
    var englishLevel: String?
    var otherLanguages: [String]
    var preferredCountries: [String]
    var preferredCities: [String]
    var preferredTimezones: [String]
    var workModes: [JobWorkMode]
    var employmentTypes: [JobEmploymentType]
    var minimumSalary: Double?
    var salaryCurrency: String?
    var salaryPeriod: SalaryPeriod?
    var visaSponsorshipRequired: Bool?
    var relocationAcceptable: Bool?
    var requiredKeywords: [String]
    var preferredKeywords: [String]
    var excludedKeywords: [String]
    var requireSalaryDisclosed: Bool
    var minimumMatchScore: Int
    var notificationEnabled: Bool
    var requiresConfirmation: Bool

    var write: JobSearchProfileWrite {
        var value = JobSearchProfileWrite()
        value.name = name; value.isDefault = isDefault; value.enabled = enabled
        value.targetRoles = targetRoles; value.excludedRoles = excludedRoles
        value.targetIndustries = targetIndustries; value.preferredSeniority = preferredSeniority
        value.candidateSkills = candidateSkills; value.yearsExperience = yearsExperience
        value.educationLevel = educationLevel; value.englishLevel = englishLevel
        value.otherLanguages = otherLanguages; value.preferredCountries = preferredCountries
        value.preferredCities = preferredCities; value.preferredTimezones = preferredTimezones
        value.workModes = workModes; value.employmentTypes = employmentTypes
        value.minimumSalary = minimumSalary; value.salaryCurrency = salaryCurrency
        value.salaryPeriod = salaryPeriod; value.visaSponsorshipRequired = visaSponsorshipRequired
        value.relocationAcceptable = relocationAcceptable; value.requiredKeywords = requiredKeywords
        value.preferredKeywords = preferredKeywords; value.excludedKeywords = excludedKeywords
        value.requireSalaryDisclosed = requireSalaryDisclosed
        value.minimumMatchScore = minimumMatchScore; value.notificationEnabled = notificationEnabled
        return value
    }
}

struct JobsPage: Codable, Sendable {
    var items: [JobOpportunity]
    var total: Int
    var limit: Int
    var offset: Int
    var filterSummary: [String: JSONValue]
    var profile: JobSearchProfile?
}

struct JobProfileParseRequest: Encodable, Sendable { var text: String }
struct JobFeedbackRequest: Encodable, Sendable { var feedbackType: JobFeedbackType; var note: String? }
struct JobFeedbackResponse: Decodable, Sendable { var id: UUID; var feedbackType: JobFeedbackType; var note: String?; var updatedAt: Date }
