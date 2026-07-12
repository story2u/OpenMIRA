import Foundation
import Observation

/// 会话状态：Keychain token + `/auth/me` 恢复；登出清空 Keychain（蓝图安全约束）。
@MainActor
@Observable
final class SessionStore {
    enum State {
        case restoring
        case restoreFailed(String)
        case loggedOut
        case active(AuthUser)
    }

    private(set) var state: State = .restoring
    let api: APIClient

    var currentUser: AuthUser? {
        if case .active(let user) = state { return user }
        return nil
    }

    init(api: APIClient = APIClient(baseURL: AppConfig.apiBaseURL, tokenProvider: { Keychain.token() })) {
        self.api = api
    }

    /// 冷启动恢复：无 token 直接登出态；401 清 token；网络错误保留 token 允许重试。
    func restore() async {
        guard Keychain.token() != nil else {
            state = .loggedOut
            return
        }
        do {
            state = .active(try await api.me())
        } catch APIError.unauthorized {
            Keychain.clearToken()
            state = .loggedOut
        } catch {
            state = .restoreFailed(error.localizedDescription)
        }
    }

    /// Sign in with Apple：用原生 id_token 换取后端 JWT。
    /// 后端端点落地前（P0 计划步骤 2）会收到 404，界面按错误展示。
    func signInWithApple(idToken: String) async throws {
        let token = try await api.nativeLogin(provider: "apple", idToken: idToken)
        Keychain.saveToken(token.accessToken)
        state = .active(token.user)
    }

    func logout() {
        Keychain.clearToken()
        state = .loggedOut
    }

    #if DEBUG
    /// ponytail: DEBUG-only 开发通道——后端原生登录端点落地前用手工签发的 JWT 调试，
    /// 不进入 Release 构建。
    func debugLogin(rawToken: String) async {
        Keychain.saveToken(rawToken.trimmingCharacters(in: .whitespacesAndNewlines))
        state = .restoring
        await restore()
    }
    #endif
}
