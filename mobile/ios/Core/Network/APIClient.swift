import Foundation

enum APIError: Error, LocalizedError {
    case unauthorized
    case networkUnavailable
    case connectionTimedOut
    case cannotReachServer
    case server(status: Int, message: String)
    case invalidResponse

    var errorDescription: String? {
        switch self {
        case .unauthorized: "登录已过期，请重新登录"
        case .networkUnavailable: "网络不可用，请检查无线局域网或蜂窝数据连接"
        case .connectionTimedOut: "连接超时，请稍后重试"
        case .cannotReachServer: "暂时无法连接服务器，请稍后重试"
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
        headers: [String: String] = [:],
        body: (some Encodable)? = nil as Never?
    ) async throws -> T {
        try Self.decode(await data(
            method: "POST",
            path: path,
            query: query,
            headers: headers,
            body: Self.encode(body)
        ))
    }

    func patch<T: Decodable>(_ path: String, body: some Encodable) async throws -> T {
        try Self.decode(await data(method: "PATCH", path: path, query: [], body: Self.encode(body)))
    }

    func delete(_ path: String) async throws {
        _ = try await data(method: "DELETE", path: path, query: [], body: nil)
    }

    // MARK: - 传输

    private func data(
        method: String,
        path: String,
        query: [URLQueryItem],
        headers: [String: String] = [:],
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
        for (field, value) in headers {
            request.setValue(value, forHTTPHeaderField: field)
        }

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch let error as URLError {
            switch error.code {
            case .cancelled:
                throw CancellationError()
            case .notConnectedToInternet, .networkConnectionLost, .dataNotAllowed,
                 .internationalRoamingOff:
                throw APIError.networkUnavailable
            case .timedOut:
                throw APIError.connectionTimedOut
            default:
                throw APIError.cannotReachServer
            }
        }
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
