import Foundation

enum AppConfig {
    static var revenueCatIOSPublicAPIKey: String? {
        let raw = Bundle.main.object(forInfoDictionaryKey: "RevenueCatIOSPublicAPIKey") as? String
        return raw?.trimmingCharacters(in: .whitespacesAndNewlines).nilIfEmpty
    }
    /// API 根地址（含 /api/v1）。
    /// Release：读 Info.plist 的 RadarAPIBaseURL（TestFlight/App Store 用生产地址）。
    /// DEBUG：默认本地 compose 后端；可在 Xcode scheme 里用环境变量 RADAR_API_URL 覆盖
    /// （例如指向线上联调），不读 Info.plist，避免本地开发误连生产。
    static var apiBaseURL: URL {
        #if DEBUG
        if let raw = ProcessInfo.processInfo.environment["RADAR_API_URL"],
           !raw.isEmpty, let url = URL(string: raw) {
            return url
        }
        return URL(string: "http://127.0.0.1:8000/api/v1")!
        #else
        guard let raw = Bundle.main.object(forInfoDictionaryKey: "RadarAPIBaseURL") as? String,
              !raw.isEmpty, let url = URL(string: raw)
        else {
            fatalError("RadarAPIBaseURL 未配置：发布构建必须在 Info.plist 注入 API 根地址")
        }
        return url
        #endif
    }
}

private extension String {
    var nilIfEmpty: String? { isEmpty ? nil : self }
}
