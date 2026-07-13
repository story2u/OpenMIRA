import Foundation

/// 商机看板筛选条件与排序。语义对齐 Web dashboard-filters.ts，但"今天"用真实设备时区
/// 计算 UTC 边界（不使用 Web 的 MOCK_NOW）。
struct DashboardQuery: Equatable, Sendable {
    enum Sort: String, CaseIterable, Identifiable, Sendable {
        case newest, oldest, confidence, trust
        var id: String { rawValue }
        var label: String {
            switch self {
            case .newest: String(localized: "sort.newest", defaultValue: "最新优先")
            case .oldest: String(localized: "sort.oldest", defaultValue: "最早优先")
            case .confidence: String(localized: "sort.confidence", defaultValue: "按相关度")
            case .trust: String(localized: "sort.trust", defaultValue: "按可信度")
            }
        }
    }

    enum TimeRange: String, CaseIterable, Identifiable, Sendable {
        case all, today, threeDays = "3d", sevenDays = "7d", custom
        var id: String { rawValue }
        var label: String {
            switch self {
            case .all: String(localized: "time.all", defaultValue: "全部时间")
            case .today: String(localized: "time.today", defaultValue: "今天")
            case .threeDays: String(localized: "time.3d", defaultValue: "近 3 天")
            case .sevenDays: String(localized: "time.7d", defaultValue: "近 7 天")
            case .custom: String(localized: "time.custom", defaultValue: "自定义")
            }
        }
    }

    enum Source: String, CaseIterable, Identifiable, Sendable {
        case all, group, `private`
        var id: String { rawValue }
        var label: String {
            switch self {
            case .all: String(localized: "source.all", defaultValue: "全部来源")
            case .group: String(localized: "source.group", defaultValue: "群消息")
            case .private: String(localized: "source.private", defaultValue: "私聊消息")
            }
        }
    }

    var status: FrontendOpportunityStatus?
    var platform: IMChannel?
    var source: Source = .all
    var timeRange: TimeRange = .all
    var customFrom: Date?
    var customTo: Date?
    var trustLevels: Set<TrustLevel> = []
    var stages: Set<SopStage> = []
    var keywords: Set<String> = []
    var sort: Sort = .newest
    /// 用户时区（来自 /settings/me workSchedule.timezone），用于"今天"边界。
    var timezoneIdentifier: String = TimeZone.current.identifier

    /// 高级筛选启用数量（与 Web countActiveAdvancedFilters 同口径）。
    var activeAdvancedCount: Int {
        var n = 0
        if source != .all { n += 1 }
        if timeRange != .all { n += 1 }
        if !keywords.isEmpty { n += 1 }
        if !trustLevels.isEmpty { n += 1 }
        if !stages.isEmpty { n += 1 }
        return n
    }

    private var timezone: TimeZone { TimeZone(identifier: timezoneIdentifier) ?? .current }

    /// 把 timeRange 解析成 UTC 起止时刻。返回 (from, to)。
    var resolvedDateBounds: (from: Date?, to: Date?) {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timezone
        let now = Date()
        switch timeRange {
        case .all:
            return (nil, nil)
        case .today:
            return (calendar.startOfDay(for: now), nil)
        case .threeDays:
            return (calendar.date(byAdding: .day, value: -3, to: now), nil)
        case .sevenDays:
            return (calendar.date(byAdding: .day, value: -7, to: now), nil)
        case .custom:
            let from = customFrom.map { calendar.startOfDay(for: $0) }
            let to = customTo.map { day -> Date in
                let start = calendar.startOfDay(for: day)
                return calendar.date(byAdding: DateComponents(day: 1, second: -1), to: start) ?? day
            }
            return (from, to)
        }
    }

    /// 自定义区间是否有效（起不晚于止）。
    var customRangeValid: Bool {
        guard timeRange == .custom, let from = customFrom, let to = customTo else { return true }
        return from <= to
    }

    /// 旧版服务端仅支持 status/platform/分页，并固定按最新排序。
    /// 只有能保持筛选语义时才允许降级，避免悄悄忽略用户选择。
    var supportsLegacyEndpoint: Bool {
        source == .all
            && timeRange == .all
            && trustLevels.isEmpty
            && stages.isEmpty
            && keywords.isEmpty
            && sort == .newest
    }

    var queryItems: [URLQueryItem] {
        var items: [URLQueryItem] = [
            URLQueryItem(name: "sort", value: sort.rawValue),
        ]
        if let status { items.append(URLQueryItem(name: "status", value: status.rawValue)) }
        if let platform { items.append(URLQueryItem(name: "platform", value: platform.rawValue)) }
        if source != .all { items.append(URLQueryItem(name: "source_type", value: source.rawValue)) }
        let bounds = resolvedDateBounds
        let iso = ISO8601DateFormatter()
        if let from = bounds.from { items.append(URLQueryItem(name: "created_from", value: iso.string(from: from))) }
        if let to = bounds.to { items.append(URLQueryItem(name: "created_to", value: iso.string(from: to))) }
        for level in trustLevels.sorted(by: { $0.rawValue < $1.rawValue }) {
            items.append(URLQueryItem(name: "trust_levels", value: level.rawValue))
        }
        for stage in stages.sorted(by: { $0.rawValue < $1.rawValue }) {
            items.append(URLQueryItem(name: "sop_stages", value: stage.rawValue))
        }
        for keyword in keywords.sorted() {
            items.append(URLQueryItem(name: "keywords", value: keyword))
        }
        return items
    }
}
