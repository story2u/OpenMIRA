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

    private enum PasswordLoginError: LocalizedError {
        case invalidCredentials

        var errorDescription: String? { "邮箱或密码错误" }
    }

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

    /// 已有密码账户用邮箱和密码换取 JWT；凭据本身不落盘。
    func signIn(email: String, password: String) async throws {
        do {
            let token = try await api.passwordLogin(
                email: email.trimmingCharacters(in: .whitespacesAndNewlines),
                password: password
            )
            Keychain.saveToken(token.accessToken)
            state = .active(token.user)
        } catch APIError.unauthorized {
            throw PasswordLoginError.invalidCredentials
        }
    }

    func logout() {
        Keychain.clearToken()
        state = .loggedOut
    }
}
