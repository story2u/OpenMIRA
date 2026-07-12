import Observation
import SwiftUI

@MainActor
@Observable
final class OpportunityDetailModel {
    private let api: APIClient
    private let operatorID: String
    let opportunityID: UUID

    var detail: Opportunity?
    var messages: [ChatMessage] = []
    var replyText = ""
    var isLoading = false
    var isSending = false
    var isDrafting = false
    var errorMessage: String?
    var templates: [ReplyTemplate] = []

    init(api: APIClient, opportunityID: UUID, operatorID: String) {
        self.api = api
        self.opportunityID = opportunityID
        self.operatorID = operatorID
    }

    func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            async let detail = api.opportunity(id: opportunityID)
            async let messages = api.messages(opportunityID: opportunityID)
            self.detail = try await detail
            self.messages = try await messages
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    /// 发送失败只报错、不改状态（验收标准：不得伪造已回复）。
    func sendReply() async {
        let text = replyText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        isSending = true
        defer { isSending = false }
        do {
            detail = try await api.sendManualReply(
                opportunityID: opportunityID,
                text: text,
                operatorID: operatorID
            )
            replyText = ""
            messages = (try? await api.messages(opportunityID: opportunityID)) ?? messages
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    /// 额度耗尽等错误直接展示后端 detail（fail-closed，验收标准要求明确提示）。
    func generateDraft() async {
        isDrafting = true
        defer { isDrafting = false }
        do {
            replyText = try await api.generateAIDraft(opportunityID: opportunityID).draft
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    /// 非法状态迁移由后端 409 拒绝，错误原样展示（验收标准）。
    func setStatus(_ status: OpportunityStatus) async {
        do {
            detail = try await api.updateStatus(opportunityID: opportunityID, to: status)
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func claim() async {
        do {
            detail = try await api.claim(opportunityID: opportunityID, operatorID: operatorID)
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func loadTemplatesIfNeeded() async {
        guard templates.isEmpty else { return }
        templates = (try? await api.templates()) ?? []
    }
}

struct OpportunityDetailView: View {
    @Environment(SessionStore.self) private var session
    let opportunityID: UUID
    @State private var model: OpportunityDetailModel?
    @State private var showTemplates = false

    var body: some View {
        Group {
            if let model {
                DetailContent(model: model, showTemplates: $showTemplates)
            } else {
                ProgressView()
            }
        }
        .navigationTitle("商机详情")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            if model == nil {
                model = OpportunityDetailModel(
                    api: session.api,
                    opportunityID: opportunityID,
                    operatorID: session.currentUser?.displayName ?? "operator"
                )
                await model?.load()
            }
        }
    }
}

private struct DetailContent: View {
    @Bindable var model: OpportunityDetailModel
    @Binding var showTemplates: Bool

    var body: some View {
        List {
            if let detail = model.detail {
                summarySection(detail)
                agentSection(detail)
                messagesSection
            } else if model.isLoading {
                HStack { Spacer(); ProgressView(); Spacer() }
            }
        }
        .refreshable { await model.load() }
        .safeAreaInset(edge: .bottom) { replyBar }
        .toolbar { statusMenu }
        .sheet(isPresented: $showTemplates) { templatePicker }
        .alert("操作失败", isPresented: .init(
            get: { model.errorMessage != nil },
            set: { if !$0 { model.errorMessage = nil } }
        )) {
            Button("好", role: .cancel) {}
        } message: {
            Text(model.errorMessage ?? "")
        }
    }

    // MARK: 概要

    private func summarySection(_ detail: Opportunity) -> some View {
        Section("概要") {
            LabeledContent("联系人", value: detail.contactName)
            LabeledContent("渠道", value: detail.platform.label)
            if let group = detail.groupName {
                LabeledContent("群组", value: group)
            }
            LabeledContent("状态", value: detail.internalStatus.label)
            LabeledContent("优先级", value: detail.priority.label)
            LabeledContent("置信度", value: detail.confidenceScore.formatted(.percent.precision(.fractionLength(0))))
            if !detail.matchedKeywords.isEmpty {
                LabeledContent("命中关键词", value: detail.matchedKeywords.joined(separator: "、"))
            }
            if let reason = detail.detectionReason {
                LabeledContent("识别依据", value: reason)
            }
        }
    }

    // MARK: Agent 发现

    @ViewBuilder
    private func agentSection(_ detail: Opportunity) -> some View {
        Section("Agent 发现（\(detail.agentAnalysisStatus.label)）") {
            if detail.attentionRequired {
                Label("重大商机，需要关注", systemImage: "exclamationmark.circle.fill")
                    .foregroundStyle(.red)
            }
            if let error = detail.agentAnalysisError {
                Label(error, systemImage: "xmark.octagon")
                    .foregroundStyle(.orange)
            }
            ForEach(detail.linkVerification.sorted(by: { $0.key < $1.key }), id: \.key) { key, value in
                LabeledContent("链接核验 · \(key)", value: value.displayText)
            }
            ForEach(detail.extractedContacts.sorted(by: { $0.key < $1.key }), id: \.key) { key, value in
                LabeledContent("联系方式 · \(key)", value: value.displayText)
            }
            ForEach(Array(detail.agentActions.enumerated()), id: \.offset) { _, action in
                VStack(alignment: .leading, spacing: 4) {
                    HStack {
                        Text(action.actionType.label).font(.subheadline.weight(.semibold))
                        if action.requiresApproval {
                            Badge(text: "需人工批准", color: .orange)
                        }
                    }
                    Text(action.reason).font(.footnote).foregroundStyle(.secondary)
                    if let draft = action.draft, !draft.isEmpty {
                        Text(draft).font(.footnote).padding(6)
                            .background(.quaternary, in: RoundedRectangle(cornerRadius: 6))
                    }
                }
            }
            if detail.agentActions.isEmpty && detail.linkVerification.isEmpty
                && detail.extractedContacts.isEmpty && !detail.attentionRequired {
                Text("暂无 Agent 发现").foregroundStyle(.secondary)
            }
        }
    }

    // MARK: 消息历史

    private var messagesSection: some View {
        Section("消息历史") {
            if model.messages.isEmpty {
                Text("暂无消息").foregroundStyle(.secondary)
            }
            ForEach(model.messages) { message in
                VStack(alignment: message.isFromContact ? .leading : .trailing, spacing: 2) {
                    HStack(spacing: 4) {
                        Text(message.senderName).font(.caption.weight(.medium))
                        if message.source == .ai {
                            Badge(text: "AI", color: .purple)
                        }
                        Text(message.sentAt, format: .dateTime.month().day().hour().minute())
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                    Text(message.content)
                        .padding(8)
                        .background(
                            message.isFromContact ? Color(.systemGray6) : Color.accentColor.opacity(0.15),
                            in: RoundedRectangle(cornerRadius: 10)
                        )
                }
                .frame(maxWidth: .infinity, alignment: message.isFromContact ? .leading : .trailing)
            }
        }
    }

    // MARK: 回复栏

    private var replyBar: some View {
        VStack(spacing: 8) {
            if let draft = model.detail?.aiReplyDraft, model.replyText.isEmpty, !draft.isEmpty {
                Button {
                    model.replyText = draft
                } label: {
                    Label("使用已有 AI 草稿", systemImage: "sparkles")
                        .font(.footnote)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            HStack(alignment: .bottom, spacing: 8) {
                TextField("输入回复…", text: $model.replyText, axis: .vertical)
                    .lineLimit(1...4)
                    .textFieldStyle(.roundedBorder)
                Button {
                    showTemplates = true
                    Task { await model.loadTemplatesIfNeeded() }
                } label: {
                    Image(systemName: "doc.on.doc")
                }
                Button {
                    Task { await model.generateDraft() }
                } label: {
                    if model.isDrafting {
                        ProgressView()
                    } else {
                        Image(systemName: "sparkles")
                    }
                }
                .disabled(model.isDrafting)
                Button {
                    Task { await model.sendReply() }
                } label: {
                    if model.isSending {
                        ProgressView()
                    } else {
                        Image(systemName: "paperplane.fill")
                    }
                }
                .disabled(model.isSending || model.replyText.trimmingCharacters(in: .whitespaces).isEmpty)
            }
        }
        .padding(12)
        .background(.bar)
    }

    // MARK: 状态操作

    private var statusMenu: some ToolbarContent {
        ToolbarItem(placement: .topBarTrailing) {
            Menu {
                Button("认领给我") { Task { await model.claim() } }
                Divider()
                Button("标记跟进") { Task { await model.setStatus(.following) } }
                Button("忽略") { Task { await model.setStatus(.ignored) } }
                Button("关闭", role: .destructive) { Task { await model.setStatus(.closed) } }
            } label: {
                Image(systemName: "ellipsis.circle")
            }
        }
    }

    // MARK: 模板选择

    private var templatePicker: some View {
        NavigationStack {
            List(model.templates) { template in
                Button {
                    model.replyText = template.content
                    showTemplates = false
                } label: {
                    VStack(alignment: .leading, spacing: 4) {
                        HStack {
                            Text(template.title).font(.headline)
                            Badge(text: template.category, color: .blue)
                        }
                        Text(template.content)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                            .lineLimit(3)
                    }
                }
                .foregroundStyle(.primary)
            }
            .overlay {
                if model.templates.isEmpty {
                    ContentUnavailableView("暂无模板", systemImage: "doc.on.doc")
                }
            }
            .navigationTitle("回复模板")
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDetents([.medium, .large])
    }
}
