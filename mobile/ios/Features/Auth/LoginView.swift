import AuthenticationServices
import SwiftUI

/// P0 登录页：Sign in with Apple。
/// Google Sign-In 需要引入官方 SPM 依赖，作为登录切片的后续子任务（见 P0 计划步骤 3）。
struct LoginView: View {
    @Environment(SessionStore.self) private var session
    @State private var errorMessage: String?
    @State private var isSigningIn = false
    #if DEBUG
    @State private var debugToken = ""
    #endif

    var body: some View {
        VStack(spacing: 24) {
            Spacer()
            Image(systemName: "dot.radiowaves.left.and.right")
                .font(.system(size: 56))
                .foregroundStyle(.tint)
            Text("商机雷达")
                .font(.largeTitle.bold())
            Text("Telegram / 企业微信商机，推送到手，随手处理")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            Spacer()

            SignInWithAppleButton(.signIn) { request in
                request.requestedScopes = [.fullName, .email]
            } onCompletion: { result in
                handleApple(result)
            }
            .frame(height: 50)
            .disabled(isSigningIn)

            #if DEBUG
            DisclosureGroup("开发调试登录") {
                SecureField("粘贴后端签发的 JWT", text: $debugToken)
                    .textFieldStyle(.roundedBorder)
                Button("使用该 token 登录") {
                    Task { await session.debugLogin(rawToken: debugToken) }
                }
                .disabled(debugToken.trimmingCharacters(in: .whitespaces).isEmpty)
            }
            .font(.footnote)
            #endif
        }
        .padding(24)
        .alert("登录失败", isPresented: .init(
            get: { errorMessage != nil },
            set: { if !$0 { errorMessage = nil } }
        )) {
            Button("好", role: .cancel) {}
        } message: {
            Text(errorMessage ?? "")
        }
    }

    private func handleApple(_ result: Result<ASAuthorization, Error>) {
        switch result {
        case .failure(let error):
            if (error as? ASAuthorizationError)?.code != .canceled {
                errorMessage = error.localizedDescription
            }
        case .success(let authorization):
            guard let credential = authorization.credential as? ASAuthorizationAppleIDCredential,
                  let tokenData = credential.identityToken,
                  let idToken = String(data: tokenData, encoding: .utf8)
            else {
                errorMessage = "未取得 Apple 身份令牌"
                return
            }
            isSigningIn = true
            Task {
                defer { isSigningIn = false }
                do {
                    try await session.signInWithApple(idToken: idToken)
                } catch {
                    errorMessage = error.localizedDescription
                }
            }
        }
    }
}
