import type { ChatMessage, Opportunity } from '@/lib/types'

export const DEMO_CLOCK = '2026-07-13T09:30:00+08:00'

const emptyContacts = {
  phone: null,
  email: null,
  telegramHandle: null,
  wecomId: null,
  extractionSource: null,
} as const

const base = {
  contactAvatar: '',
  groupMemberRole: 'member',
  rawMessageLinks: [],
  linkVerification: { status: 'safe', verifiedAt: DEMO_CLOCK, riskReasons: [], resolvedInfo: null },
  extractedContacts: emptyContacts,
  friendRequestStatus: 'not_sent',
  agentActions: [],
  agentAnalysisStatus: 'completed',
  agentAnalysisError: null,
  agentAnalyzedAt: DEMO_CLOCK,
  attentionRequired: false,
  archivedAt: null,
  archivedByUserId: null,
  archiveReason: null,
} satisfies Partial<Opportunity>

function opportunity(item: Partial<Opportunity> & Pick<Opportunity, 'id' | 'platform' | 'contactName' | 'summary'>): Opportunity {
  return {
    ...base,
    matchedKeywords: [],
    confidenceScore: 0.75,
    status: 'pending',
    priority: 'normal',
    lastMessagePreview: item.summary,
    createdAt: '2026-07-13T09:00:00+08:00',
    sourceType: 'group',
    groupName: '产品需求交流站（演示）',
    sopStage: 'analyzing',
    trustScore: 76,
    ...item,
  } as Opportunity
}

export const demoOpportunities: Opportunity[] = [
  opportunity({
    id: 'demo-procurement-50', platform: 'telegram', contactName: '林远（演示）',
    summary: '我们团队想采购 50 套设备，下周能安排演示吗？预算已确认，希望本月完成供应商评估。',
    matchedKeywords: ['采购 50 套', '安排演示', '本月评估'], confidenceScore: 0.96, trustScore: 91,
    priority: 'urgent', groupName: '企业采购需求站（演示）', sopStage: 'contact_extracted', attentionRequired: true,
    extractedContacts: { ...emptyContacts, email: 'procurement@example.com', telegramHandle: '@demo_buyer', extractionSource: 'message_text' },
    agentActions: [
      { actionType: 'private_message', reason: '确认演示时间与设备规格', target: '@demo_buyer', draft: '您好，我们可以安排演示。请问下周二或周三哪天方便？', requiresApproval: true },
      { actionType: 'notify_user', reason: '采购数量和时间窗口明确', target: null, draft: null, requiresApproval: false },
    ],
  }),
  opportunity({
    id: 'demo-api-rfp', platform: 'telegram', contactName: '周屿（演示）',
    summary: '寻找客服自动化 API 服务商，需要 CRM 对接和 SLA，年度预算 40–60 万。',
    matchedKeywords: ['API', 'SLA', '年度预算'], confidenceScore: 0.93, trustScore: 88,
    priority: 'urgent', groupName: 'SaaS 方案交流站（演示）', sopStage: 'verified', attentionRequired: true,
    rawMessageLinks: ['https://rfp.example.com/customer-api'],
    linkVerification: { status: 'safe', verifiedAt: DEMO_CLOCK, riskReasons: [], resolvedInfo: 'example.com 演示域名；RFP 字段完整，未发现跳转风险。' },
  }),
  opportunity({
    id: 'demo-private-deploy', platform: 'telegram', contactName: '许澄（演示）',
    summary: '私聊咨询私有化部署与数据隔离，计划两周内完成技术选型。',
    matchedKeywords: ['私有化部署', '数据隔离'], confidenceScore: 0.87, trustScore: 94,
    priority: 'high', sourceType: 'private', groupName: null, sopStage: 'ready_to_chat',
  }),
  opportunity({
    id: 'demo-wecom-renewal', platform: 'wecom', contactName: '顾言（演示）',
    summary: '现有客户询问年度续约折扣，并计划增加 30 个使用席位。',
    matchedKeywords: ['续约', '增购席位'], confidenceScore: 0.91, trustScore: 97,
    priority: 'high', sourceType: 'private', groupName: null, status: 'replied', sopStage: 'chatting',
  }),
  opportunity({
    id: 'demo-risk-link', platform: 'wecom', contactName: '陆青（演示）',
    summary: '群内分享外部“订单资源”页面，来源和跳转目标尚未完成核验。',
    matchedKeywords: ['订单资源', '合作'], confidenceScore: 0.72, trustScore: 34,
    priority: 'high', groupName: '渠道合作讨论站（演示）', sopStage: 'analyzing',
    rawMessageLinks: ['https://unverified.example.com/leads'],
    linkVerification: { status: 'unverified', verifiedAt: null, riskReasons: ['等待安全读取器核验'], resolvedInfo: null },
  }),
  opportunity({
    id: 'demo-multilingual', platform: 'telegram', contactName: 'Mina Demo',
    summary: '跨境团队咨询中日韩多语言客服自动化方案，希望先试用再评估。',
    matchedKeywords: ['多语言', '试用'], confidenceScore: 0.82, trustScore: 79,
    priority: 'normal', groupName: '跨境产品交流站（演示）', sopStage: 'detected',
  }),
  opportunity({ id: 'demo-training', platform: 'wecom', contactName: '沈禾（演示）', summary: '计划采购团队培训服务，正在比较三家供应商。', matchedKeywords: ['采购', '供应商比较'], confidenceScore: 0.79, trustScore: 84, sourceType: 'private', groupName: null }),
  opportunity({ id: 'demo-integration', platform: 'telegram', contactName: '陈曜（演示）', summary: '需要把消息系统接入内部 CRM，询问实施周期。', matchedKeywords: ['CRM 接入', '实施周期'], confidenceScore: 0.85, trustScore: 82, groupName: '工程协作站（演示）' }),
  opportunity({ id: 'demo-consulting', platform: 'wecom', contactName: '唐宁（演示）', summary: '寻求数据治理咨询，下季度启动，预算审批中。', matchedKeywords: ['咨询', '预算审批'], confidenceScore: 0.76, trustScore: 71, priority: 'normal', status: 'replied', sourceType: 'private', groupName: null }),
  opportunity({ id: 'demo-low', platform: 'telegram', contactName: '韩知（演示）', summary: '询问产品资料，暂未给出预算和时间。', matchedKeywords: ['产品资料'], confidenceScore: 0.54, trustScore: 67, priority: 'low', groupName: '产品观察站（演示）' }),
  opportunity({ id: 'demo-ignored', platform: 'wecom', contactName: '宋溪（演示）', summary: '重复推广信息，已由运营标记忽略。', matchedKeywords: ['推广'], confidenceScore: 0.41, trustScore: 29, priority: 'low', status: 'ignored', groupName: '行业分享站（演示）', sopStage: 'closed' }),
  opportunity({ id: 'demo-pilot', platform: 'telegram', contactName: '叶川（演示）', summary: '希望为 120 人团队启动一个月试点，要求提供评估指标。', matchedKeywords: ['120 人', '试点'], confidenceScore: 0.89, trustScore: 86, priority: 'high', groupName: 'AI 落地交流站（演示）', sopStage: 'verified' }),
]

export const demoMessages: Record<string, ChatMessage[]> = {
  'demo-procurement-50': [
    { id: 'm1', senderName: '林远（演示）', content: '我们团队正在做设备更新，本批预计采购 50 套。', isFromContact: true, sentAt: '2026-07-13T08:54:00+08:00', source: 'human' },
    { id: 'm2', senderName: '林远（演示）', content: '下周能安排演示吗？预算已经确认，希望本月完成供应商评估。', isFromContact: true, sentAt: '2026-07-13T08:56:00+08:00', source: 'human' },
  ],
}

export const featuredDemoOpportunity = demoOpportunities[0]
