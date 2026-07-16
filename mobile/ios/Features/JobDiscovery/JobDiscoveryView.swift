import Observation
import SwiftUI

@MainActor
@Observable
final class JobDiscoveryViewModel {
    private let api: APIClient
    var jobs: [JobOpportunity] = []
    var profiles: [JobSearchProfile] = []
    var selectedProfileID: UUID?
    var query = ""
    var source: IMChannel?
    var workMode: JobWorkMode?
    var minimumMatchScore = 0
    var isLoading = false
    var errorMessage: String?
    var total = 0

    init(api: APIClient) { self.api = api }

    func load() async {
        isLoading = true
        errorMessage = nil
        do {
            profiles = try await api.jobSearchProfiles()
            if selectedProfileID == nil { selectedProfileID = profiles.first?.id }
            var items = [URLQueryItem(name: "sort", value: "match")]
            if let selectedProfileID { items.append(.init(name: "profile_id", value: selectedProfileID.uuidString)) }
            if !query.isEmpty { items.append(.init(name: "query", value: query)) }
            if let source { items.append(.init(name: "source", value: source.rawValue)) }
            if let workMode { items.append(.init(name: "work_mode", value: workMode.rawValue)) }
            if minimumMatchScore > 0 { items.append(.init(name: "minimum_match_score", value: String(minimumMatchScore))) }
            let page = try await api.jobs(query: items)
            jobs = page.items
            total = page.total
        } catch {
            errorMessage = error.localizedDescription
        }
        isLoading = false
    }
}

struct JobDiscoveryView: View {
    @Environment(SessionStore.self) private var session
    @State private var model: JobDiscoveryViewModel?
    @State private var selection: UUID?
    @State private var showsFilters = false

    var body: some View {
        NavigationSplitView {
            Group {
                if let model { jobList(model) } else { ProgressView() }
            }
            .navigationTitle("工作机会")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("筛选", systemImage: "line.3.horizontal.decrease.circle") { showsFilters = true }
                }
            }
        } detail: {
            if let selection { JobDetailView(jobID: selection) }
            else { ContentUnavailableView("选择一个工作机会", systemImage: "briefcase") }
        }
        .sheet(isPresented: $showsFilters) {
            if let model { JobFilterSheet(model: model) }
        }
        .task {
            if model == nil {
                let value = JobDiscoveryViewModel(api: session.api)
                model = value
                await value.load()
            }
        }
    }

    private func jobList(_ model: JobDiscoveryViewModel) -> some View {
        List(selection: $selection) {
            Section {
                Picker("求职档案", selection: Binding(
                    get: { model.selectedProfileID },
                    set: { model.selectedProfileID = $0; Task { await model.load() } }
                )) {
                    Text("未选择").tag(UUID?.none)
                    ForEach(model.profiles) { profile in Text(profile.name).tag(UUID?.some(profile.id)) }
                }
                .pickerStyle(.menu)
            }
            if let error = model.errorMessage {
                Section { ContentUnavailableView("加载失败", systemImage: "wifi.exclamationmark", description: Text(error)) }
            }
            Section("共 \(model.total) 个匹配职位") {
                ForEach(model.jobs) { job in
                    JobRow(job: job).tag(job.id)
                }
            }
        }
        .overlay { if model.isLoading && model.jobs.isEmpty { ProgressView() } }
        .refreshable { await model.load() }
    }
}

private struct JobRow: View {
    let job: JobOpportunity
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(job.jobTitle).font(.headline)
                    Text(job.companyName ?? "公司未说明").font(.subheadline).foregroundStyle(.secondary)
                }
                Spacer()
                if let score = job.match?.matchScore { Text("\(score)").font(.title3.bold()).foregroundStyle(AppColors.primary).accessibilityLabel("匹配分 \(score)") }
            }
            HStack(spacing: 10) {
                Label(job.locationText ?? "地点未说明", systemImage: "mappin")
                Text(job.workMode.label)
                Text(job.salaryRaw ?? "薪资未公开")
            }
            .font(.caption).foregroundStyle(.secondary).lineLimit(1)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack { ForEach(job.requiredSkills.prefix(4), id: \.self) { Text($0).font(.caption2).padding(.horizontal, 7).padding(.vertical, 3).background(.quaternary, in: Capsule()) } }
            }
            HStack {
                Label(job.sourceChatName ?? job.sourceChannel.label, systemImage: "dot.radiowaves.left.and.right")
                Spacer()
                Text(job.postedAt, style: .date)
            }.font(.caption2).foregroundStyle(.secondary)
            if !job.complianceFlags.isEmpty { Label("招聘原文包含合规风险提示", systemImage: "exclamationmark.triangle").font(.caption).foregroundStyle(AppColors.warning) }
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
    }
}

private struct JobFilterSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Bindable var model: JobDiscoveryViewModel
    var body: some View {
        NavigationStack {
            Form {
                TextField("岗位、公司或技能", text: $model.query)
                Picker("来源", selection: $model.source) {
                    Text("全部").tag(IMChannel?.none)
                    Text("Telegram").tag(IMChannel?.some(.telegram))
                    Text("企业微信").tag(IMChannel?.some(.wecom))
                }
                Picker("工作模式", selection: $model.workMode) {
                    Text("不限").tag(JobWorkMode?.none)
                    ForEach(JobWorkMode.allCases.filter { $0 != .unknown }, id: \.self) { Text($0.label).tag(JobWorkMode?.some($0)) }
                }
                Stepper("最低匹配分 \(model.minimumMatchScore)", value: $model.minimumMatchScore, in: 0...100, step: 5)
            }
            .navigationTitle("筛选工作机会")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("应用") { dismiss(); Task { await model.load() } } }
            }
        }
        .presentationDetents([.medium, .large])
    }
}
