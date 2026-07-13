import SwiftUI

/// 设置中心：原生分组列表。顺序对齐 Web settings/page.tsx。
struct SettingsView: View {
    @Environment(SessionStore.self) private var session
    @State private var model: SettingsViewModel?

    var body: some View {
        NavigationStack {
            Group {
                if let model {
                    SettingsList(model: model)
                } else {
                    ProgressView()
                }
            }
            .navigationTitle(Text(String(localized: "settings.title", defaultValue: "设置")))
        }
        .task {
            if model == nil {
                let vm = SettingsViewModel(api: session.api)
                model = vm
                await vm.load()
            }
        }
    }
}

private struct SettingsList: View {
    @Environment(SessionStore.self) private var session
    @Bindable var model: SettingsViewModel

    var body: some View {
        List {
            userHeader

            if let error = model.loadError, model.bundle == nil {
                Section {
                    ContentUnavailableView {
                        Label(String(localized: "state.load_failed", defaultValue: "加载失败"), systemImage: "wifi.exclamationmark")
                    } description: {
                        Text(error)
                    } actions: {
                        Button(String(localized: "action.retry", defaultValue: "重试")) { Task { await model.load() } }
                    }
                }
            }

            if model.serverRequiresUpgrade, model.bundle == nil {
                Section {
                    Label {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(String(localized: "settings.upgrade_required", defaultValue: "设置同步暂不可用"))
                                .font(.headline)
                            Text(String(localized: "settings.upgrade_required_detail", defaultValue: "当前服务端版本较旧，升级后即可管理识别规则、工作时间和通知偏好。"))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    } icon: {
                        Image(systemName: "server.rack")
                            .foregroundStyle(AppColors.warning)
                    }
                }
            }

            Section {
                if let user = session.currentUser {
                    NavigationLink {
                        SubscriptionView(api: session.api, billing: session.billing, userID: user.id)
                    } label: {
                        settingsRow(icon: "creditcard", tint: AppColors.ai, title: String(localized: "settings.subscription", defaultValue: "套餐与用量"))
                    }
                }
                NavigationLink {
                    TelegramSettingsView()
                } label: {
                    settingsRow(icon: "paperplane", tint: AppColors.telegram, title: String(localized: "settings.telegram", defaultValue: "Telegram 连接"))
                }
                wecomRow
            } header: {
                Text(String(localized: "settings.section.binding", defaultValue: "平台绑定"))
            }

            Section {
                if let model = model.bundle {
                    NavigationLink {
                        DetectionSettingsView(model: self.model, detection: model.detection)
                    } label: {
                        settingsRow(icon: "tag", tint: AppColors.primary, title: String(localized: "settings.detection", defaultValue: "商机识别规则"))
                    }
                    NavigationLink {
                        WorkScheduleView(model: self.model, schedule: model.workSchedule)
                    } label: {
                        settingsRow(icon: "clock", tint: AppColors.warning, title: String(localized: "settings.work_hours", defaultValue: "工作时间"))
                    }
                    NavigationLink {
                        NotificationSettingsView(model: self.model, notifications: model.notifications, pushAvailable: model.capabilities.pushAvailable)
                    } label: {
                        settingsRow(icon: "bell", tint: AppColors.destructive, title: String(localized: "settings.notifications", defaultValue: "通知设置"))
                    }
                } else if model.isLoading {
                    HStack { Spacer(); ProgressView(); Spacer() }
                }
            } header: {
                Text(String(localized: "settings.section.automation", defaultValue: "识别与自动化"))
            }

            Section {
                Button(role: .destructive) {
                    session.logout()
                } label: {
                    Label(String(localized: "settings.logout", defaultValue: "退出登录"), systemImage: "rectangle.portrait.and.arrow.right")
                }
            }
        }
        .refreshable { await model.load() }
    }

    private var userHeader: some View {
        Section {
            HStack(spacing: 12) {
                ZStack {
                    Circle().fill(AppColors.primary.opacity(0.15))
                    Text(String(session.currentUser?.displayName.prefix(1) ?? "我"))
                        .font(.title3.weight(.semibold))
                        .foregroundStyle(AppColors.primary)
                }
                .frame(width: 48, height: 48)
                VStack(alignment: .leading, spacing: 2) {
                    Text(session.currentUser?.displayName ?? "—").font(.headline)
                    Text(session.currentUser?.email ?? "").font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
            }
            .padding(.vertical, 4)
        }
    }

    private var wecomRow: some View {
        // 企业微信无用户级绑定：诚实标注"由管理员统一配置"，不做假连接入口。
        HStack {
            settingsRow(icon: "building.2", tint: AppColors.wecom, title: String(localized: "settings.wecom", defaultValue: "企业微信"))
            Spacer()
            Text(String(localized: "settings.wecom_admin", defaultValue: "由管理员配置"))
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    private func settingsRow(icon: String, tint: Color, title: String) -> some View {
        Label {
            Text(title)
        } icon: {
            Image(systemName: icon)
                .foregroundStyle(tint)
        }
    }
}
