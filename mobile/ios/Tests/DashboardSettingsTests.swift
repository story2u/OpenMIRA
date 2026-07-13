import XCTest
@testable import OpportunityRadar

/// 看板与设置的 DTO 解码 + DashboardQuery 查询参数生成（含时区边界与筛选）。
final class DashboardSettingsTests: XCTestCase {
    private func decoder() -> JSONDecoder { APIClient.makeDecoder() }

    func testDashboardResponseDecoding() throws {
        let json = """
        {
          "items": [],
          "total": 42,
          "limit": 20,
          "offset": 0,
          "pendingCount": 7,
          "attentionItems": [],
          "keywordOptions": ["报价", "采购"]
        }
        """.data(using: .utf8)!
        let response = try decoder().decode(DashboardResponse.self, from: json)
        XCTAssertEqual(response.total, 42)
        XCTAssertEqual(response.pendingCount, 7)
        XCTAssertEqual(response.keywordOptions, ["报价", "采购"])
    }

    func testSettingsBundleDecoding() throws {
        let json = """
        {
          "detection": {"keywords": ["报价"], "aiSemanticsEnabled": true},
          "workSchedule": {
            "timezone": "Asia/Shanghai",
            "slots": [{"weekday": 1, "start": "09:00", "end": "18:00"}],
            "autoReplyOutsideHours": true,
            "isDefault": false
          },
          "notifications": {
            "newOpportunityEnabled": true, "aiRepliedEnabled": false,
            "dailyDigestEnabled": false, "urgentOnly": true
          },
          "capabilities": {"pushAvailable": false, "wecomUserBindingAvailable": false}
        }
        """.data(using: .utf8)!
        let bundle = try decoder().decode(SettingsBundle.self, from: json)
        XCTAssertEqual(bundle.detection.keywords, ["报价"])
        XCTAssertEqual(bundle.workSchedule.slots.first?.weekday, 1)
        XCTAssertTrue(bundle.notifications.urgentOnly)
        XCTAssertFalse(bundle.capabilities.pushAvailable)
    }

    func testQueryItemsIncludeArraysAndSort() {
        var query = DashboardQuery()
        query.status = .pending
        query.platform = .telegram
        query.source = .group
        query.trustLevels = [.trusted, .risky]
        query.stages = [.detected]
        query.keywords = ["报价"]
        query.sort = .confidence

        let items = query.queryItems
        func values(_ name: String) -> [String] {
            items.filter { $0.name == name }.compactMap(\.value)
        }
        XCTAssertEqual(values("status"), ["pending"])
        XCTAssertEqual(values("platform"), ["telegram"])
        XCTAssertEqual(values("source_type"), ["group"])
        XCTAssertEqual(values("sort"), ["confidence"])
        XCTAssertEqual(Set(values("trust_levels")), ["trusted", "risky"])
        XCTAssertEqual(values("sop_stages"), ["detected"])
        XCTAssertEqual(values("keywords"), ["报价"])
    }

    func testTimeRangeTodayUsesTimezoneBoundary() {
        var query = DashboardQuery()
        query.timeRange = .today
        query.timezoneIdentifier = "Asia/Shanghai"
        let bounds = query.resolvedDateBounds
        XCTAssertNotNil(bounds.from)
        XCTAssertNil(bounds.to)
        // 上海时区的今天零点转成 UTC 后，秒数应落在 16:00 UTC（前一天）附近。
        if let from = bounds.from {
            var cal = Calendar(identifier: .gregorian)
            cal.timeZone = TimeZone(identifier: "Asia/Shanghai")!
            let comps = cal.dateComponents([.hour, .minute, .second], from: from)
            XCTAssertEqual(comps.hour, 0)
            XCTAssertEqual(comps.minute, 0)
        }
    }

    func testCustomRangeValidation() {
        var query = DashboardQuery()
        query.timeRange = .custom
        query.customFrom = Date(timeIntervalSince1970: 2_000_000)
        query.customTo = Date(timeIntervalSince1970: 1_000_000)
        XCTAssertFalse(query.customRangeValid)
        query.customTo = Date(timeIntervalSince1970: 3_000_000)
        XCTAssertTrue(query.customRangeValid)
    }

    func testActiveAdvancedCount() {
        var query = DashboardQuery()
        XCTAssertEqual(query.activeAdvancedCount, 0)
        query.source = .group
        query.trustLevels = [.trusted]
        XCTAssertEqual(query.activeAdvancedCount, 2)
    }

    func testLegacyEndpointCompatibilityRequiresEquivalentQuery() {
        var query = DashboardQuery()
        XCTAssertTrue(query.supportsLegacyEndpoint)

        query.status = .pending
        query.platform = .telegram
        XCTAssertTrue(query.supportsLegacyEndpoint)

        query.sort = .confidence
        XCTAssertFalse(query.supportsLegacyEndpoint)
    }

    func testNavigationTitleDefaultsAreChinese() {
        XCTAssertEqual(String(localized: "dashboard.title", defaultValue: "商机"), "商机")
        XCTAssertEqual(String(localized: "settings.title", defaultValue: "设置"), "设置")
    }

    func testTrustLevelBoundaries() {
        XCTAssertEqual(TrustLevel.from(score: 80), .trusted)
        XCTAssertEqual(TrustLevel.from(score: 79), .unverified)
        XCTAssertEqual(TrustLevel.from(score: 60), .unverified)
        XCTAssertEqual(TrustLevel.from(score: 59), .suspicious)
        XCTAssertEqual(TrustLevel.from(score: 40), .suspicious)
        XCTAssertEqual(TrustLevel.from(score: 39), .risky)
    }
}
