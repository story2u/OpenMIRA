import Foundation

enum AppConfig {
    /// API 根地址（含 /api/v1）。发布配置经 Info.plist 的 RadarAPIBaseURL 注入；
    /// DEBUG 默认指向本地 compose 后端。
    static var apiBaseURL: URL {
        if let raw = Bundle.main.object(forInfoDictionaryKey: "RadarAPIBaseURL") as? String,
           !raw.isEmpty, let url = URL(string: raw) {
            return url
        }
        #if DEBUG
        return URL(string: "http://127.0.0.1:8000/api/v1")!
        #else
        fatalError("RadarAPIBaseURL 未配置：发布构建必须在 Info.plist 注入 API 根地址")
        #endif
    }
}
