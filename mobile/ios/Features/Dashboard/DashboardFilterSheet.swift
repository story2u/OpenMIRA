import SwiftUI

/// 高级筛选 Bottom Sheet。语义对齐 Web filter-panel.tsx：时间范围/来源/可信度/流程阶段/关键词。
/// 采用草稿模式：编辑不即时生效，点"应用"才写回，"取消"丢弃。
struct DashboardFilterSheet: View {
    @Binding var query: DashboardQuery
    let keywordOptions: [String]
    @Environment(\.dismiss) private var dismiss

    @State private var draft: DashboardQuery

    init(query: Binding<DashboardQuery>, keywordOptions: [String]) {
        _query = query
        self.keywordOptions = keywordOptions
        _draft = State(initialValue: query.wrappedValue)
    }

    var body: some View {
        NavigationStack {
            Form {
                timeRangeSection
                if draft.timeRange == .custom { customRangeSection }
                sourceSection
                trustSection
                stageSection
                if !keywordOptions.isEmpty { keywordSection }
            }
            .navigationTitle(Text("filter.title", bundle: .main))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "action.cancel", defaultValue: "取消")) { dismiss() }
                }
                ToolbarItem(placement: .principal) {
                    if draft.activeAdvancedCount > 0 {
                        Text(String(localized: "filter.active_count", defaultValue: "\(draft.activeAdvancedCount) 项筛选"))
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "action.apply", defaultValue: "应用")) {
                        query = draft
                        dismiss()
                    }
                    .disabled(!draft.customRangeValid)
                }
                ToolbarItem(placement: .bottomBar) {
                    Button(String(localized: "action.reset", defaultValue: "重置"), role: .destructive) {
                        draft = resetAdvanced(draft)
                    }
                }
            }
        }
    }

    private var timeRangeSection: some View {
        Section(String(localized: "filter.time_range", defaultValue: "时间范围")) {
            Picker(String(localized: "filter.time_range", defaultValue: "时间范围"), selection: $draft.timeRange) {
                ForEach(DashboardQuery.TimeRange.allCases) { Text($0.label).tag($0) }
            }
            .pickerStyle(.menu)
        }
    }

    private var customRangeSection: some View {
        Section {
            DatePicker(
                String(localized: "filter.custom_from", defaultValue: "开始日期"),
                selection: Binding($draft.customFrom, default: Date()),
                displayedComponents: .date
            )
            DatePicker(
                String(localized: "filter.custom_to", defaultValue: "结束日期"),
                selection: Binding($draft.customTo, default: Date()),
                displayedComponents: .date
            )
            if !draft.customRangeValid {
                Label(
                    String(localized: "filter.custom_invalid", defaultValue: "开始日期不能晚于结束日期"),
                    systemImage: "exclamationmark.triangle"
                )
                .font(.caption)
                .foregroundStyle(AppColors.destructive)
            }
        }
    }

    private var sourceSection: some View {
        Section(String(localized: "filter.source", defaultValue: "消息来源")) {
            Picker(String(localized: "filter.source", defaultValue: "消息来源"), selection: $draft.source) {
                ForEach(DashboardQuery.Source.allCases) { Text($0.label).tag($0) }
            }
            .pickerStyle(.menu)
        }
    }

    private var trustSection: some View {
        Section(String(localized: "filter.trust", defaultValue: "可信度")) {
            ForEach(TrustLevel.allCases) { level in
                multiToggle(
                    label: level.label,
                    color: level.semanticColor,
                    isOn: draft.trustLevels.contains(level)
                ) { on in
                    if on { draft.trustLevels.insert(level) } else { draft.trustLevels.remove(level) }
                }
            }
        }
    }

    private var stageSection: some View {
        Section(String(localized: "filter.stage", defaultValue: "流程阶段")) {
            ForEach(SopStage.allCases) { stage in
                multiToggle(
                    label: stage.label,
                    color: stage.dotColor,
                    isOn: draft.stages.contains(stage)
                ) { on in
                    if on { draft.stages.insert(stage) } else { draft.stages.remove(stage) }
                }
            }
        }
    }

    private var keywordSection: some View {
        Section {
            ForEach(keywordOptions, id: \.self) { keyword in
                multiToggle(label: keyword, color: AppColors.primary, isOn: draft.keywords.contains(keyword)) { on in
                    if on { draft.keywords.insert(keyword) } else { draft.keywords.remove(keyword) }
                }
            }
        } header: {
            HStack {
                Text(String(localized: "filter.keywords", defaultValue: "关键词标签"))
                Spacer()
                if !draft.keywords.isEmpty {
                    Button(String(localized: "action.clear", defaultValue: "清空")) { draft.keywords.removeAll() }
                        .font(.caption)
                }
            }
        }
    }

    private func multiToggle(
        label: String,
        color: Color,
        isOn: Bool,
        onChange: @escaping (Bool) -> Void
    ) -> some View {
        Button {
            onChange(!isOn)
        } label: {
            HStack {
                Circle().fill(color).frame(width: 8, height: 8)
                Text(label).foregroundStyle(.primary)
                Spacer()
                if isOn { Image(systemName: "checkmark").foregroundStyle(AppColors.primary) }
            }
        }
        .accessibilityAddTraits(isOn ? [.isSelected] : [])
    }

    private func resetAdvanced(_ q: DashboardQuery) -> DashboardQuery {
        var next = q
        next.source = .all
        next.timeRange = .all
        next.customFrom = nil
        next.customTo = nil
        next.trustLevels = []
        next.stages = []
        next.keywords = []
        return next
    }
}

private extension Binding where Value: Sendable {
    /// Optional Date 与非可选 DatePicker 的桥接。
    init(_ source: Binding<Value?>, default defaultValue: Value) {
        self.init(
            get: { source.wrappedValue ?? defaultValue },
            set: { source.wrappedValue = $0 }
        )
    }
}
