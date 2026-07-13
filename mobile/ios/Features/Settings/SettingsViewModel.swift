import Observation
import SwiftUI

@MainActor
@Observable
final class SettingsViewModel {
    private let api: APIClient

    var bundle: SettingsBundle?
    var isLoading = false
    var loadError: String?
    var serverRequiresUpgrade = false

    init(api: APIClient) {
        self.api = api
    }

    func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            bundle = try await api.settings()
            loadError = nil
            serverRequiresUpgrade = false
        } catch APIError.server(let status, _) where status == 404 {
            // 老版本服务端尚未注册 /settings/me；不要把它误报成网络故障。
            loadError = nil
            serverRequiresUpgrade = true
        } catch {
            // 加载失败不显示默认值冒充服务端值。
            loadError = error.localizedDescription
            serverRequiresUpgrade = false
        }
    }

    // MARK: 识别规则

    /// 乐观更新 + 失败回滚：立即改本地，失败时恢复旧值并抛错给 UI 提示。
    func saveDetection(keywords: [String], aiSemanticsEnabled: Bool) async throws {
        guard var current = bundle else { return }
        let previous = current.detection
        current.detection = DetectionSettings(keywords: keywords, aiSemanticsEnabled: aiSemanticsEnabled)
        bundle = current
        do {
            let saved = try await api.updateDetectionSettings(
                DetectionSettingsUpdate(keywords: keywords, aiSemanticsEnabled: aiSemanticsEnabled)
            )
            bundle?.detection = saved
        } catch {
            bundle?.detection = previous
            throw error
        }
    }

    // MARK: 工作时间

    func saveWorkSchedule(_ schedule: WorkSchedule) async throws {
        guard let current = bundle else { return }
        let previous = current.workSchedule
        bundle?.workSchedule = schedule
        do {
            let saved = try await api.updateWorkSchedule(
                WorkScheduleUpdate(
                    timezone: schedule.timezone,
                    slots: schedule.slots,
                    autoReplyOutsideHours: schedule.autoReplyOutsideHours
                )
            )
            bundle?.workSchedule = saved
        } catch {
            bundle?.workSchedule = previous
            throw error
        }
    }

    // MARK: 通知

    func saveNotifications(_ prefs: NotificationSettings) async throws {
        guard let current = bundle else { return }
        let previous = current.notifications
        bundle?.notifications = prefs
        do {
            let saved = try await api.updateNotificationSettings(
                NotificationSettingsUpdate(
                    newOpportunityEnabled: prefs.newOpportunityEnabled,
                    aiRepliedEnabled: prefs.aiRepliedEnabled,
                    dailyDigestEnabled: prefs.dailyDigestEnabled,
                    urgentOnly: prefs.urgentOnly
                )
            )
            bundle?.notifications = saved
        } catch {
            bundle?.notifications = previous
            throw error
        }
    }
}
