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
            MainTabView()
        }
    }
}

/// 三个一级 Tab：商机看板 / 工作机会 / 设置中心。每个 Tab 内部各自持有导航状态，
/// 切 Tab 保留各自的导航栈、滚动位置、筛选与已加载数据（SwiftUI TabView 默认行为）。
struct MainTabView: View {
    var body: some View {
        TabView {
            DashboardView()
                .tabItem {
                    Label(String(localized: "tab.dashboard", defaultValue: "商机"), systemImage: "tray.full")
                }
            JobDiscoveryView()
                .tabItem {
                    Label("工作机会", systemImage: "briefcase")
                }
            SettingsView()
                .tabItem {
                    Label(String(localized: "tab.settings", defaultValue: "设置"), systemImage: "gearshape")
                }
        }
    }
}
