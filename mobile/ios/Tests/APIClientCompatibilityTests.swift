import Foundation
import XCTest
@testable import OpportunityRadar

private final class StubURLProtocol: URLProtocol, @unchecked Sendable {
    nonisolated(unsafe) static var handler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        guard let handler = Self.handler else {
            client?.urlProtocol(self, didFailWithError: URLError(.unknown))
            return
        }
        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}

final class APIClientCompatibilityTests: XCTestCase {
    override func tearDown() {
        StubURLProtocol.handler = nil
        super.tearDown()
    }

    func testDashboardFallsBackToLegacyListWhenNewRouteIsMissing() async throws {
        nonisolated(unsafe) var requestedPaths: [String] = []
        StubURLProtocol.handler = { request in
            let path = request.url!.path
            requestedPaths.append(path)
            switch path {
            case "/api/v1/opportunities/dashboard":
                return Self.response(request, status: 422, json: #"{"detail":[{"msg":"invalid UUID"}]}"#)
            case "/api/v1/opportunities":
                return Self.response(request, status: 200, json: "[]")
            default:
                return Self.response(request, status: 404, json: #"{"detail":"Not Found"}"#)
            }
        }

        let page = try await makeAPI().dashboard(
            DashboardQueryPaged(query: DashboardQuery(), limit: 20, offset: 0)
        )

        XCTAssertTrue(page.usesLegacyAPI)
        XCTAssertEqual(page.response.items.count, 0)
        XCTAssertEqual(
            requestedPaths,
            ["/api/v1/opportunities/dashboard", "/api/v1/opportunities"]
        )
    }

    @MainActor
    func testSettings404IsReportedAsServerUpgradeInsteadOfNetworkFailure() async {
        StubURLProtocol.handler = { request in
            Self.response(request, status: 404, json: #"{"detail":"Not Found"}"#)
        }
        let model = SettingsViewModel(api: makeAPI())

        await model.load()

        XCTAssertTrue(model.serverRequiresUpgrade)
        XCTAssertNil(model.loadError)
    }

    func testOfflineTransportHasActionableChineseMessage() async {
        StubURLProtocol.handler = { _ in throw URLError(.notConnectedToInternet) }

        do {
            let _: AuthUser = try await makeAPI().me()
            XCTFail("Expected networkUnavailable")
        } catch APIError.networkUnavailable {
            XCTAssertEqual(APIError.networkUnavailable.errorDescription, "网络不可用，请检查无线局域网或蜂窝数据连接")
        } catch {
            XCTFail("Unexpected error: \(error)")
        }
    }

    private func makeAPI() -> APIClient {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StubURLProtocol.self]
        return APIClient(
            baseURL: URL(string: "https://example.test/api/v1")!,
            tokenProvider: { nil },
            session: URLSession(configuration: configuration)
        )
    }

    private static func response(
        _ request: URLRequest,
        status: Int,
        json: String
    ) -> (HTTPURLResponse, Data) {
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: status,
            httpVersion: nil,
            headerFields: ["Content-Type": "application/json"]
        )!
        return (response, Data(json.utf8))
    }
}
