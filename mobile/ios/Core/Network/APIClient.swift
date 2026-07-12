import Foundation

enum APIError: Error, LocalizedError {
    case unauthorized
    case server(status: Int, message: String)
    case invalidResponse

    var errorDescription: String? {
        switch self {
        case .unauthorized: "登录已过期，请重新登录"
        case .server(_, let message): message
        case .invalidResponse: "服务返回了无法解析的数据"
        }
    }
}

/// 唯一 HTTP 边界：所有后端访问都经过这里（蓝图约束，等同 frontend/lib/api.ts）。
final class APIClient: Sendable {
    private let baseURL: URL
    private let tokenProvider: @Sendable () -> String?
    private let session: URLSession

    init(
        baseURL: URL,
        tokenProvider: @escaping @Sendable () -> String?,
        session: URLSession = .shared
    ) {
        self.baseURL = baseURL
        self.tokenProvider = tokenProvider
        self.session = session
    }

    // MARK: - 类型化入口

    func get<T: Decodable>(_ path: String, query: [URLQueryItem] = []) async throws -> T {
        try Self.decode(await data(method: "GET", path: path, query: query, body: nil))
    }

    func post<T: Decodable>(
        _ path: String,
        query: [URLQueryItem] = [],
        body: (some Encodable)? = nil as Never?
    ) async throws -> T {
        try Self.decode(await data(method: "POST", path: path, query: query, body: Self.encode(body)))
    }

    func patch<T: Decodable>(_ path: String, body: some Encodable) async throws -> T {
        try Self.decode(await data(method: "PATCH", path: path, query: [], body: Self.encode(body)))
    }

    // MARK: - 传输

    private func data(
        method: String,
        path: String,
        query: [URLQueryItem],
        body: Data?
    ) async throws -> Data {
        var components = URLComponents(
            url: baseURL.appending(path: path),
            resolvingAgainstBaseURL: false
        )!
        if !query.isEmpty {
            components.queryItems = query
        }
        var request = URLRequest(url: components.url!)
        request.httpMethod = method
        request.httpBody = body
        if body != nil {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        if let token = tokenProvider() {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else { throw APIError.invalidResponse }
        switch http.statusCode {
        case 200..<300:
            return data
        case 401:
            throw APIError.unauthorized
        default:
            throw APIError.server(status: http.statusCode, message: Self.errorMessage(from: data, status: http.statusCode))
        }
    }

    // MARK: - 编解码

    private static func encode(_ body: (some Encodable)?) throws -> Data? {
        guard let body else { return nil }
        return try JSONEncoder().encode(body)
    }

    private static func decode<T: Decodable>(_ data: Data) throws -> T {
        try makeDecoder().decode(T.self, from: data)
    }

    /// 后端时间为 ISO8601（多数带小数秒）；测试复用同一 decoder 保证契约一致。
    static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let raw = try decoder.singleValueContainer().decode(String.self)
            let formatter = ISO8601DateFormatter()
            formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = formatter.date(from: raw) { return date }
            formatter.formatOptions = [.withInternetDateTime]
            if let date = formatter.date(from: raw) { return date }
            throw DecodingError.dataCorrupted(.init(
                codingPath: decoder.codingPath,
                debugDescription: "无法解析时间: \(raw)"
            ))
        }
        return decoder
    }

    private static func errorMessage(from data: Data, status: Int) -> String {
        struct Detail: Decodable { let detail: String }
        if let parsed = try? JSONDecoder().decode(Detail.self, from: data) {
            return parsed.detail
        }
        return "请求失败（HTTP \(status)）"
    }
}
