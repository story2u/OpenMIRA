import AuthenticationServices
import SwiftUI

/// 登录页：邮箱密码和 Sign in with Apple。
struct LoginView: View {
    @Environment(SessionStore.self) private var session
    @State private var email = ""
    @State private var password = ""
    @State private var errorMessage: String?
    @State private var activeLoginMethod: LoginMethod?
    @FocusState private var focusedField: Field?

    private enum LoginMethod {
        case password
        case apple
    }

    private enum Field {
        case email
        case password
    }

    private var canSubmitPassword: Bool {
        let normalizedEmail = email.trimmingCharacters(in: .whitespacesAndNewlines)
        let emailParts = normalizedEmail.split(separator: "@", omittingEmptySubsequences: false)
        let emailLooksValid = emailParts.count == 2 && emailParts[1].contains(".")
        return emailLooksValid
            && normalizedEmail.count <= 320
            && !password.isEmpty
            && password.count <= 128
            && activeLoginMethod == nil
    }

    var body: some View {
        VStack(spacing: 24) {
            Spacer()
            RadarLogoView()
            Text("商机雷达")
                .font(.largeTitle.bold())
            Text("Telegram / 企业微信商机，推送到手，随手处理")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            Spacer()

            VStack(spacing: 14) {
                TextField("邮箱", text: $email)
                    .textContentType(.username)
                    .keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .submitLabel(.next)
                    .focused($focusedField, equals: .email)
                    .onSubmit { focusedField = .password }
                    .textFieldStyle(.roundedBorder)

                SecureField("密码", text: $password)
                    .textContentType(.password)
                    .submitLabel(.go)
                    .focused($focusedField, equals: .password)
                    .onSubmit { submitPasswordLogin() }
                    .textFieldStyle(.roundedBorder)

                Button(action: submitPasswordLogin) {
                    Group {
                        if activeLoginMethod == .password {
                            ProgressView()
                                .tint(.white)
                        } else {
                            Text("使用邮箱登录")
                        }
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .disabled(!canSubmitPassword)
            }

            HStack {
                Rectangle()
                    .frame(height: 1)
                    .foregroundStyle(.tertiary)
                Text("或")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                Rectangle()
                    .frame(height: 1)
                    .foregroundStyle(.tertiary)
            }

            SignInWithAppleButton(.signIn) { request in
                request.requestedScopes = [.fullName, .email]
            } onCompletion: { result in
                handleApple(result)
            }
            .frame(height: 50)
            .disabled(activeLoginMethod != nil)
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

    private func submitPasswordLogin() {
        guard canSubmitPassword else { return }
        focusedField = nil
        activeLoginMethod = .password
        Task {
            defer { activeLoginMethod = nil }
            do {
                try await session.signIn(email: email, password: password)
            } catch {
                errorMessage = error.localizedDescription
            }
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
            activeLoginMethod = .apple
            Task {
                defer { activeLoginMethod = nil }
                do {
                    try await session.signInWithApple(idToken: idToken)
                } catch {
                    errorMessage = error.localizedDescription
                }
            }
        }
    }
}
