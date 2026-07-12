import SwiftUI

@main
struct OpportunityRadarApp: App {
    @State private var session = SessionStore()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(session)
        }
    }
}

struct RootView: View {
    @Environment(SessionStore.self) private var session

    var body: some View {
        switch session.state {
        case .restoring:
            ProgressView("正在恢复会话…")
                .task { await session.restore() }
        case .restoreFailed(let message):
            ContentUnavailableView {
                Label("会话恢复失败", systemImage: "wifi.exclamationmark")
            } description: {
                Text(message)
            } actions: {
                Button("重试") { Task { await session.restore() } }
                Button("退出登录", role: .destructive) { session.logout() }
            }
        case .loggedOut:
            LoginView()
        case .active:
            InboxView()
        }
    }
}
