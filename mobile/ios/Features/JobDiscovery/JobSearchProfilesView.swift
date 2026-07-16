import Observation
import SwiftUI

@MainActor
@Observable
final class JobSearchProfilesViewModel {
    private let api: APIClient
    var profiles: [JobSearchProfile] = []
    var errorMessage: String?
    var isLoading = false
    init(api: APIClient) { self.api = api }
    func load() async { isLoading = true; defer { isLoading = false }; do { profiles = try await api.jobSearchProfiles() } catch { errorMessage = error.localizedDescription } }
    func save(id: UUID?, draft: JobSearchProfileWrite) async -> Bool {
        do { if let id { _ = try await api.updateJobSearchProfile(id: id, body: draft) } else { _ = try await api.createJobSearchProfile(draft) }; await load(); return true }
        catch { errorMessage = error.localizedDescription; return false }
    }
    func delete(id: UUID) async { do { try await api.deleteJobSearchProfile(id: id); await load() } catch { errorMessage = error.localizedDescription } }
    func parse(_ text: String) async -> JobSearchProfilePreview? { do { return try await api.parseJobSearchProfile(text) } catch { errorMessage = error.localizedDescription; return nil } }
}

struct JobSearchProfilesView: View {
    @Environment(SessionStore.self) private var session
    @State private var model: JobSearchProfilesViewModel?
    @State private var editor: EditorState?
    @State private var naturalText = ""
    @State private var isParsing = false

    var body: some View {
        List {
            Section("自然语言生成预览") {
                TextField("远程 Python 后端，欧洲时区…", text: $naturalText, axis: .vertical)
                Button("生成预览", systemImage: "sparkles") { Task { await parse() } }.disabled(naturalText.count < 5 || isParsing)
            }
            if let error = model?.errorMessage { Section { Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(AppColors.destructive) } }
            Section("求职档案") {
                if model?.isLoading == true { ProgressView() }
                ForEach(model?.profiles ?? []) { profile in
                    Button { editor = .init(id: profile.id, draft: .init(profile), confirmationRequired: false) } label: {
                        HStack { VStack(alignment: .leading) { Text(profile.name); Text(profile.targetRoles.joined(separator: "、")).font(.caption).foregroundStyle(.secondary) }; Spacer(); if profile.isDefault { Text("默认").font(.caption).foregroundStyle(AppColors.primary) }; Image(systemName: "chevron.right").foregroundStyle(.tertiary) }
                    }.buttonStyle(.plain)
                    .swipeActions { Button("删除", role: .destructive) { Task { await model?.delete(id: profile.id) } } }
                }
            }
        }
        .navigationTitle("求职档案")
        .toolbar { Button("新建", systemImage: "plus") { editor = .init(id: nil, draft: JobSearchProfileWrite(), confirmationRequired: false) } }
        .sheet(item: $editor) { state in
            if let model { JobSearchProfileEditor(state: state) { draft in await model.save(id: state.profileID, draft: draft) } }
        }
        .task { if model == nil { let value = JobSearchProfilesViewModel(api: session.api); model = value; await value.load() } }
    }

    private func parse() async {
        guard let model else { return }
        isParsing = true
        if let preview = await model.parse(naturalText) {
            editor = EditorState(id: nil, draft: preview.write, confirmationRequired: preview.requiresConfirmation)
        }
        isParsing = false
    }
}

private struct EditorState: Identifiable {
    let id = UUID()
    let profileID: UUID?
    var draft: JobSearchProfileWrite
    let confirmationRequired: Bool
    init(id profileID: UUID?, draft: JobSearchProfileWrite, confirmationRequired: Bool) { self.profileID = profileID; self.draft = draft; self.confirmationRequired = confirmationRequired }
}

private struct JobSearchProfileEditor: View {
    @Environment(\.dismiss) private var dismiss
    let state: EditorState
    let onSave: (JobSearchProfileWrite) async -> Bool
    @State private var draft: JobSearchProfileWrite
    @State private var confirmed: Bool
    @State private var saving = false

    init(state: EditorState, onSave: @escaping (JobSearchProfileWrite) async -> Bool) {
        self.state = state; self.onSave = onSave
        _draft = State(initialValue: state.draft)
        _confirmed = State(initialValue: !state.confirmationRequired)
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("职位目标") {
                    TextField("档案名称", text: $draft.name)
                    TextField("目标岗位（逗号分隔）", text: listBinding(\.targetRoles))
                    TextField("排除岗位", text: listBinding(\.excludedRoles))
                    TextField("技能", text: listBinding(\.candidateSkills))
                    Picker("资历", selection: firstBinding(\.preferredSeniority)) { Text("不限").tag(JobSeniority?.none); ForEach(JobSeniority.allCases.filter { $0 != .unknown }, id: \.self) { Text($0.label).tag(JobSeniority?.some($0)) } }
                    TextField("经验年限", value: $draft.yearsExperience, format: .number).keyboardType(.decimalPad)
                }
                Section("地点与工作") {
                    TextField("国家", text: listBinding(\.preferredCountries))
                    TextField("城市", text: listBinding(\.preferredCities))
                    TextField("时区", text: listBinding(\.preferredTimezones))
                    Picker("工作模式", selection: firstBinding(\.workModes)) { Text("不限").tag(JobWorkMode?.none); ForEach(JobWorkMode.allCases.filter { $0 != .unknown }, id: \.self) { Text($0.label).tag(JobWorkMode?.some($0)) } }
                    Picker("雇佣类型", selection: firstBinding(\.employmentTypes)) { Text("不限").tag(JobEmploymentType?.none); ForEach(JobEmploymentType.allCases.filter { $0 != .unknown }, id: \.self) { Text($0.label).tag(JobEmploymentType?.some($0)) } }
                }
                Section("条件") {
                    TextField("最低薪资", value: $draft.minimumSalary, format: .number).keyboardType(.decimalPad)
                    TextField("币种", text: optionalStringBinding(\.salaryCurrency))
                    Picker("签证支持", selection: $draft.visaSponsorshipRequired) { Text("不作为必要条件").tag(Bool?.none); Text("必须支持").tag(Bool?.some(true)); Text("不需要").tag(Bool?.some(false)) }
                    TextField("偏好关键词", text: listBinding(\.preferredKeywords))
                    TextField("排除关键词", text: listBinding(\.excludedKeywords))
                    Stepper("最低匹配分 \(draft.minimumMatchScore)", value: $draft.minimumMatchScore, in: 0...100, step: 5)
                }
                Section { Toggle("设为默认", isOn: $draft.isDefault); Toggle("启用档案", isOn: $draft.enabled); Toggle("要求公开薪资", isOn: $draft.requireSalaryDisclosed); Toggle("匹配通知", isOn: $draft.notificationEnabled) }
                if state.confirmationRequired { Section { Toggle("我已确认解析出的职业偏好", isOn: $confirmed) } footer: { Text("保存后才会参与职位匹配。") } }
            }
            .navigationTitle(state.profileID == nil ? "新建求职档案" : "编辑求职档案")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("保存") { Task { saving = true; if await onSave(draft) { dismiss() }; saving = false } }.disabled(draft.name.isEmpty || !confirmed || saving) }
            }
        }
    }

    private func listBinding(_ keyPath: WritableKeyPath<JobSearchProfileWrite, [String]>) -> Binding<String> {
        Binding(get: { draft[keyPath: keyPath].joined(separator: "，") }, set: { draft[keyPath: keyPath] = $0.split(whereSeparator: { ",，\n".contains($0) }).map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty } })
    }
    private func optionalStringBinding(_ keyPath: WritableKeyPath<JobSearchProfileWrite, String?>) -> Binding<String> {
        Binding(get: { draft[keyPath: keyPath] ?? "" }, set: { draft[keyPath: keyPath] = $0.isEmpty ? nil : $0 })
    }
    private func firstBinding<T>(_ keyPath: WritableKeyPath<JobSearchProfileWrite, [T]>) -> Binding<T?> {
        Binding(get: { draft[keyPath: keyPath].first }, set: { draft[keyPath: keyPath] = $0.map { [$0] } ?? [] })
    }
}
