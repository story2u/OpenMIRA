import SwiftUI

/// 工作时间：一周 7 行，每天可开关并设置人工审核时段；自动接待需另行授权。
/// 手机端不照搬桌面 560px 横向表格；用每天一行 + 时间选择，符合触控规范。
struct WorkScheduleView: View {
    let model: SettingsViewModel

    struct DayEntry: Equatable {
        var enabled: Bool
        var start: String
        var end: String
    }

    @State private var timezone: String
    @State private var days: [Int: DayEntry]  // weekday 1-7
    @State private var original: [Int: DayEntry]
    @State private var originalTimezone: String
    @State private var autoReplyOutsideHours: Bool
    @State private var originalAutoReplyOutsideHours: Bool
    @State private var isSaving = false
    @State private var errorMessage: String?

    private static let weekdayNames = ["", "周一", "周二", "周三", "周四", "周五", "周六", "周日"]
    private static let commonTimezones = [
        "Asia/Shanghai", "Asia/Hong_Kong", "Asia/Tokyo", "Asia/Singapore",
        "America/Los_Angeles", "America/New_York", "Europe/London", "UTC",
    ]

    init(model: SettingsViewModel, schedule: WorkSchedule) {
        self.model = model
        let mapped = Self.decode(schedule.slots)
        _timezone = State(initialValue: schedule.timezone)
        _originalTimezone = State(initialValue: schedule.timezone)
        _days = State(initialValue: mapped)
        _original = State(initialValue: mapped)
        _autoReplyOutsideHours = State(initialValue: schedule.autoReplyOutsideHours)
        _originalAutoReplyOutsideHours = State(initialValue: schedule.autoReplyOutsideHours)
    }

    private var hasChanges: Bool {
        days != original
            || timezone != originalTimezone
            || autoReplyOutsideHours != originalAutoReplyOutsideHours
    }

    private var weeklyHours: Int {
        days.values.filter(\.enabled).reduce(0) { total, entry in
            total + Self.hoursBetween(entry.start, entry.end)
        }
    }

    var body: some View {
        Form {
            Section {
                Picker(String(localized: "work.timezone", defaultValue: "时区"), selection: $timezone) {
                    ForEach(Self.commonTimezones, id: \.self) { Text($0).tag($0) }
                }
            } footer: {
                Text(String(localized: "work.timezone_hint", defaultValue: "日夜模式切换依据所选时区判断"))
            }

            Section {
                ForEach(1...7, id: \.self) { weekday in
                    dayRow(weekday)
                }
            } header: {
                HStack {
                    Text(String(localized: "work.schedule", defaultValue: "人工审核时段"))
                    Spacer()
                    Text(String(localized: "work.weekly_hours", defaultValue: "本周共 \(weeklyHours) 小时"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            } footer: {
                Text(String(localized: "work.footer", defaultValue: "选中时段内为人工审核；其余时段默认仍进入人工队列。"))
            }

            Section {
                Toggle(
                    String(localized: "work.auto_reply", defaultValue: "非工作时间 AI 安全接待"),
                    isOn: $autoReplyOutsideHours
                )
            } footer: {
                Text(String(localized: "work.auto_reply_hint", defaultValue: "仅对已单独授权的 Telegram Business 私聊生效，发送前仍需通过 Agent 风险检查。"))
            }

            Section {
                Button(String(localized: "work.copy_weekdays", defaultValue: "工作日统一设为 09:00–18:00")) {
                    for weekday in 1...5 { days[weekday] = DayEntry(enabled: true, start: "09:00", end: "18:00") }
                }
                Button(String(localized: "work.clear", defaultValue: "全部清空"), role: .destructive) {
                    for weekday in 1...7 { days[weekday]?.enabled = false }
                }
            }

            if let errorMessage {
                Section { Label(errorMessage, systemImage: "exclamationmark.triangle").foregroundStyle(AppColors.destructive) }
            }
        }
        .navigationTitle(Text("settings.work_hours", bundle: .main))
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button(String(localized: "action.save", defaultValue: "保存"), action: save)
                    .disabled(isSaving || !hasChanges)
            }
        }
    }

    private func dayRow(_ weekday: Int) -> some View {
        let entry = days[weekday] ?? DayEntry(enabled: false, start: "09:00", end: "18:00")
        return VStack(spacing: 8) {
            Toggle(Self.weekdayNames[weekday], isOn: Binding(
                get: { entry.enabled },
                set: { days[weekday, default: entry].enabled = $0 }
            ))
            if entry.enabled {
                HStack {
                    timePicker(selection: Binding(
                        get: { entry.start },
                        set: { days[weekday, default: entry].start = $0 }
                    ))
                    Text("–").foregroundStyle(.secondary)
                    timePicker(selection: Binding(
                        get: { entry.end },
                        set: { days[weekday, default: entry].end = $0 }
                    ))
                    Spacer()
                }
                .font(.callout)
            }
        }
    }

    private func timePicker(selection: Binding<String>) -> some View {
        // 产品范围 08:00–22:00，整点粒度。
        Picker("", selection: selection) {
            ForEach(8...22, id: \.self) { hour in
                let value = String(format: "%02d:00", hour)
                Text(value).tag(value)
            }
        }
        .labelsHidden()
        .pickerStyle(.menu)
    }

    private func save() {
        isSaving = true
        errorMessage = nil
        let slots = Self.encode(days)
        let schedule = WorkSchedule(
            timezone: timezone,
            slots: slots,
            autoReplyOutsideHours: autoReplyOutsideHours,
            isDefault: false
        )
        Task {
            do {
                try await model.saveWorkSchedule(schedule)
                original = days
                originalTimezone = timezone
                originalAutoReplyOutsideHours = autoReplyOutsideHours
            } catch {
                errorMessage = error.localizedDescription
            }
            isSaving = false
        }
    }

    // MARK: 编解码：DTO slots <-> 每天一段

    private static func decode(_ slots: [WorkScheduleSlotDTO]) -> [Int: DayEntry] {
        var result: [Int: DayEntry] = [:]
        for weekday in 1...7 { result[weekday] = DayEntry(enabled: false, start: "09:00", end: "18:00") }
        for slot in slots {
            result[slot.weekday] = DayEntry(enabled: true, start: slot.start, end: slot.end)
        }
        return result
    }

    private static func encode(_ days: [Int: DayEntry]) -> [WorkScheduleSlotDTO] {
        (1...7).compactMap { weekday in
            guard let entry = days[weekday], entry.enabled, entry.start < entry.end else { return nil }
            return WorkScheduleSlotDTO(weekday: weekday, start: entry.start, end: entry.end)
        }
    }

    private static func hoursBetween(_ start: String, _ end: String) -> Int {
        let s = Int(start.prefix(2)) ?? 0
        let e = Int(end.prefix(2)) ?? 0
        return max(0, e - s)
    }
}
