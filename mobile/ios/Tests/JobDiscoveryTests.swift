import XCTest
@testable import OpportunityRadar

final class JobDiscoveryTests: XCTestCase {
    func testJobListDecodesMatchAndUnknownConstraints() throws {
        let json = """
        {
          "items": [{
            "opportunityId":"00000000-0000-0000-0000-000000000011",
            "jobTitle":"Python Backend Engineer","companyName":"Example Labs",
            "sourceChannel":"telegram","sourceChatName":"Example Remote Jobs",
            "postedAt":"2026-07-16T08:00:00Z","locationText":"Berlin / Remote",
            "countryCode":"DE","city":"Berlin","workMode":"remote",
            "employmentType":"full_time","seniority":"mid","salaryRaw":"USD 80k/year",
            "salaryMin":80000,"salaryMax":100000,"salaryCurrency":"USD","salaryPeriod":"annual",
            "requiredSkills":["Python","FastAPI"],"degreeLevel":null,"englishLevel":"professional",
            "visaSponsorship":null,"applicationDeadline":null,"sourceReliabilityScore":0.8,
            "extractionConfidence":0.91,"sourceCount":2,"conflictingSourceData":false,
            "complianceFlags":[],"isExpired":false,
            "match":{"eligibility":"unknown","matchScore":82,"matchedReasons":["支持远程"],
              "mismatchReasons":[],"unknownConstraints":["招聘信息未说明签证支持"],
              "scoreBreakdown":{"role":25}}
          }],
          "total":1,"limit":20,"offset":0,"filterSummary":{},"profile":null
        }
        """

        let page = try APIClient.makeDecoder().decode(JobsPage.self, from: Data(json.utf8))

        XCTAssertEqual(page.items.first?.match?.matchScore, 82)
        XCTAssertEqual(page.items.first?.match?.unknownConstraints, ["招聘信息未说明签证支持"])
    }

    func testProfilePreviewRequiresExplicitConfirmationAndContainsOnlyProfessionalFields() throws {
        let json = """
        {"name":"远程后端","isDefault":false,"enabled":true,
         "targetRoles":["Python Backend Engineer"],"excludedRoles":[],"targetIndustries":[],
         "preferredSeniority":["mid"],"candidateSkills":["Python"],"yearsExperience":3,
         "educationLevel":null,"englishLevel":null,"otherLanguages":[],"preferredCountries":[],
         "preferredCities":[],"preferredTimezones":["Europe/Berlin"],"workModes":["remote"],
         "employmentTypes":["full_time"],"minimumSalary":80000,"salaryCurrency":"USD",
         "salaryPeriod":"annual","visaSponsorshipRequired":true,"relocationAcceptable":null,
         "requiredKeywords":[],"preferredKeywords":[],"excludedKeywords":[],
         "requireSalaryDisclosed":false,"minimumMatchScore":60,"notificationEnabled":false,
         "requiresConfirmation":true}
        """

        let preview = try APIClient.makeDecoder().decode(JobSearchProfilePreview.self, from: Data(json.utf8))

        XCTAssertTrue(preview.requiresConfirmation)
        XCTAssertEqual(preview.write.workModes, [.remote])
        XCTAssertEqual(preview.write.visaSponsorshipRequired, true)
    }

    func testFutureJobEnumSafelyFallsBackToUnknown() throws {
        let mode = try JSONDecoder().decode(JobWorkMode.self, from: Data("\"distributed_first\"".utf8))
        XCTAssertEqual(mode, .unknown)
    }
}
