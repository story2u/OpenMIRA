import Observation
import SwiftUI

@MainActor
@Observable
final class JobDetailViewModel {
    private let api: APIClient
    let jobID: UUID
    var job: JobOpportunityDetail?
    var errorMessage: String?
    var feedbackSent: JobFeedbackType?
    init(api: APIClient, jobID: UUID) { self.api = api; self.jobID = jobID }
    func load() async { do { job = try await api.job(id: jobID) } catch { errorMessage = error.localizedDescription } }
    func feedback(_ type: JobFeedbackType) async { do { _ = try await api.submitJobFeedback(id: jobID, type: type); feedbackSent = type } catch { errorMessage = error.localizedDescription } }
}

struct JobDetailView: View {
    @Environment(SessionStore.self) private var session
    let jobID: UUID
    @State private var model: JobDetailViewModel?
    var body: some View {
        Group {
            if let model, let job = model.job { detail(job, model: model) }
            else if let error = model?.errorMessage { ContentUnavailableView("加载失败", systemImage: "wifi.exclamationmark", description: Text(error)) }
            else { ProgressView() }
        }
        .navigationTitle("职位详情")
        .navigationBarTitleDisplayMode(.inline)
        .task { if model == nil { let value = JobDetailViewModel(api: session.api, jobID: jobID); model = value; await value.load() } }
    }

    private func detail(_ job: JobOpportunityDetail, model: JobDetailViewModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                VStack(alignment: .leading, spacing: 8) {
                    Text(job.jobTitle).font(.title2.bold())
                    Text(job.companyName ?? "公司未说明").foregroundStyle(.secondary)
                    ViewThatFits { HStack { facts(job) }; VStack(alignment: .leading) { facts(job) } }
                    if let applicationUrl = job.applicationUrl { Link(destination: applicationUrl) { Label("前往投递", systemImage: "arrow.up.right.square") }.buttonStyle(.borderedProminent) }
                }
                if let match = job.match { matchSection(match) }
                GroupBox("核心要求") {
                    VStack(alignment: .leading, spacing: 10) {
                        Text(job.requirementsSummary ?? "招聘信息未说明")
                        LabeledContent("经验", value: job.minimumYearsExperience.map { "\($0.formatted()) 年以上" } ?? "未说明")
                        LabeledContent("学历", value: job.degreeLevel ?? "未说明")
                        LabeledContent("英语", value: job.englishLevel ?? "未说明")
                        LabeledContent("签证", value: job.visaSponsorship.map { $0 ? "明确支持" : "明确不支持" } ?? "未说明")
                    }.frame(maxWidth: .infinity, alignment: .leading)
                }
                if job.ageRequirementPresent {
                    Label {
                        Text("招聘原文包含年龄限制：\(job.ageRequirementText ?? "未提供原文")。该条件可能涉及就业歧视，系统不会将用户年龄用于推荐计算。")
                    } icon: { Image(systemName: "exclamationmark.triangle") }
                    .foregroundStyle(AppColors.warning).padding().background(AppColors.warning.opacity(0.1), in: RoundedRectangle(cornerRadius: 8))
                }
                GroupBox("来源信息") {
                    VStack(alignment: .leading, spacing: 12) {
                        ForEach(job.sources) { source in
                            VStack(alignment: .leading, spacing: 4) {
                                Text("\(source.channel.label) · \(source.chatName ?? "私聊来源")").font(.subheadline.bold())
                                Text(source.authorName ?? "发布者未说明").font(.caption).foregroundStyle(.secondary)
                                if let url = source.sourceMessageUrl { Link("查看原始消息", destination: url) }
                                else { Text("该消息来自私有群组，只能在原平台查看。") .font(.caption).foregroundStyle(.secondary) }
                            }
                        }
                    }.frame(maxWidth: .infinity, alignment: .leading)
                }
                GroupBox("原始信息与字段证据") {
                    VStack(alignment: .leading, spacing: 8) {
                        Text(job.rawExcerpt)
                        Divider()
                        ForEach(job.fieldEvidence.keys.sorted(), id: \.self) { key in LabeledContent(key, value: job.fieldEvidence[key] ?? "") }
                    }.frame(maxWidth: .infinity, alignment: .leading)
                }
                GroupBox("反馈") {
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 92))]) {
                        ForEach(JobFeedbackType.allCases.filter { $0 != .unknown }, id: \.self) { type in
                            Button(type.label) { Task { await model.feedback(type) } }
                                .buttonStyle(.bordered)
                                .tint(model.feedbackSent == type ? AppColors.primary : nil)
                        }
                    }
                }
            }.padding()
        }
    }

    @ViewBuilder private func facts(_ job: JobOpportunityDetail) -> some View {
        Label(job.locationText ?? "地点未说明", systemImage: "mappin")
        Text(job.workMode.label)
        Text(job.salaryRaw ?? "薪资未公开")
    }

    private func matchSection(_ match: JobMatch) -> some View {
        GroupBox("匹配分析 · \(match.matchScore) / 100") {
            VStack(alignment: .leading, spacing: 12) {
                ProgressView(value: Double(match.matchScore), total: 100)
                MatchReasons(title: "符合", icon: "checkmark.circle", items: match.matchedReasons, color: AppColors.success)
                MatchReasons(title: "不符合", icon: "xmark.circle", items: match.mismatchReasons, color: AppColors.destructive)
                MatchReasons(title: "信息缺失", icon: "questionmark.circle", items: match.unknownConstraints, color: AppColors.warning)
            }.frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

private struct MatchReasons: View {
    let title: String; let icon: String; let items: [String]; let color: Color
    var body: some View { VStack(alignment: .leading, spacing: 3) { Label(title, systemImage: icon).foregroundStyle(color).font(.subheadline.bold()); ForEach(items, id: \.self) { Text("• \($0)").font(.caption).foregroundStyle(.secondary) } } }
}
