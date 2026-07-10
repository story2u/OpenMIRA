import type { ChatMessage, ExtractedContacts, LinkVerification, Opportunity, ReplyTemplate, SopStage } from './types'

type OpportunitySeed = Omit<
  Opportunity,
  'agentActions' | 'agentAnalysisStatus' | 'agentAnalysisError' | 'agentAnalyzedAt' | 'attentionRequired'
>

function withAgentDefaults(opportunity: OpportunitySeed): Opportunity {
  return {
    ...opportunity,
    agentActions: [],
    agentAnalysisStatus: 'not_requested',
    agentAnalysisError: null,
    agentAnalyzedAt: null,
    attentionRequired: false,
  }
}

const noLinks: LinkVerification = {
  status: 'unverified',
  verifiedAt: null,
  riskReasons: [],
  resolvedInfo: null,
}

const emptyContacts: ExtractedContacts = {
  phone: null,
  email: null,
  telegramHandle: null,
  wecomId: null,
  extractionSource: null,
}

const seedOpportunities: OpportunitySeed[] = [
  // ===== 核心演示数据 1：链接安全、流程正常推进（群消息） =====
  {
    id: 'opp-safe',
    platform: 'telegram',
    contactName: 'Michael Chen',
    contactAvatar: '/avatars/michael.png',
    summary: '群内发布企业级 API 采购需求，附需求文档链接，预计团队规模 200 人',
    matchedKeywords: ['API 接入', '批量采购', '企业版'],
    confidenceScore: 0.94,
    status: 'pending',
    priority: 'urgent',
    lastMessagePreview: '需求细节都在这个文档里，有意向的服务商可以联系我。',
    createdAt: '2026-07-07T09:32:00+08:00',
    sourceType: 'group',
    groupName: 'SaaS 出海交流群',
    groupMemberRole: 'member',
    rawMessageLinks: ['https://saas-procure.example.com/rfp/2026-api'],
    linkVerification: {
      status: 'safe',
      verifiedAt: '2026-07-07T09:35:00+08:00',
      riskReasons: [],
      resolvedInfo:
        '落地页为正规企业采购需求文档（RFP）：寻求客服自动化 API 服务商，团队 200 人，预算区间 ¥40-60 万/年，要求支持 CRM 对接与 SLA 保障，联系窗口开放至 7 月 20 日。',
    },
    extractedContacts: {
      phone: null,
      email: 'procurement@huayan-tech.com',
      telegramHandle: '@michael_chen_biz',
      wecomId: null,
      extractionSource: 'link_content',
    },
    friendRequestStatus: 'pending',
    sopStage: 'friend_requested',
    trustScore: 88,
  },
  // ===== 核心演示数据 2：链接可疑、流程被中断（群消息） =====
  {
    id: 'opp-risk',
    platform: 'wecom',
    contactName: '刘晓峰',
    contactAvatar: '/avatars/xiaofeng.png',
    summary: '群内声称有"千万级企业订单资源"，附外部链接，要求点击注册领取',
    matchedKeywords: ['企业订单', '合作'],
    confidenceScore: 0.71,
    status: 'pending',
    priority: 'high',
    lastMessagePreview: '点这个链接注册就能对接大客户资源，名额有限！',
    createdAt: '2026-07-07T08:58:00+08:00',
    sourceType: 'group',
    groupName: '华南企业服务资源对接群',
    groupMemberRole: 'unknown',
    rawMessageLinks: ['http://biz-order-hub.xyz/reg?invite=8827'],
    linkVerification: {
      status: 'suspicious',
      verifiedAt: '2026-07-07T09:02:00+08:00',
      riskReasons: [
        '域名注册时间仅 11 天，无备案信息',
        '页面要求填写手机号与验证码，疑似钓鱼站点',
        '页面内容与"企业订单资源"描述不符，实际为推广注册页',
      ],
      resolvedInfo: null,
    },
    extractedContacts: emptyContacts,
    friendRequestStatus: 'not_sent',
    sopStage: 'analyzing',
    trustScore: 23,
  },
  // ===== 私聊来源：已可直接对话 =====
  {
    id: 'opp-1',
    platform: 'telegram',
    contactName: '王丽娜',
    contactAvatar: '/avatars/lina.png',
    summary: '私聊咨询私有化部署方案，关注数据安全合规认证情况',
    matchedKeywords: ['私有化部署', '数据安全'],
    confidenceScore: 0.87,
    status: 'pending',
    priority: 'high',
    lastMessagePreview: '你们有等保三级认证吗？私有化部署大概什么周期？',
    createdAt: '2026-07-07T08:45:00+08:00',
    sourceType: 'private',
    groupName: null,
    groupMemberRole: 'member',
    rawMessageLinks: [],
    linkVerification: noLinks,
    extractedContacts: {
      phone: '138****6621',
      email: null,
      telegramHandle: '@linawang_hx',
      wecomId: null,
      extractionSource: 'message_text',
    },
    friendRequestStatus: 'n/a',
    sopStage: 'chatting',
    trustScore: 92,
  },
  // ===== 群消息：已核验完成、待提取联系方式 =====
  {
    id: 'opp-2',
    platform: 'telegram',
    contactName: 'Sarah Kim',
    contactAvatar: '/avatars/sarah.png',
    summary: '群内询问跨境电商多语言客服自动化方案，未留联系方式',
    matchedKeywords: ['多语言', '自动化', '跨境电商'],
    confidenceScore: 0.78,
    status: 'pending',
    priority: 'normal',
    lastMessagePreview: 'Any vendors here support Korean and Japanese customer replies?',
    createdAt: '2026-07-06T23:12:00+08:00',
    sourceType: 'group',
    groupName: 'Cross-border Sellers Hub',
    groupMemberRole: 'member',
    rawMessageLinks: [],
    linkVerification: noLinks,
    extractedContacts: emptyContacts,
    friendRequestStatus: 'not_sent',
    sopStage: 'verified',
    trustScore: 64,
  },
  // ===== 私聊：已回复、沟通中 =====
  {
    id: 'opp-3',
    platform: 'wecom',
    contactName: '张建国',
    contactAvatar: '/avatars/jianguo.png',
    summary: '老客户咨询年度续约折扣，并希望增购 50 个坐席',
    matchedKeywords: ['续约', '增购', '折扣'],
    confidenceScore: 0.91,
    status: 'replied',
    priority: 'high',
    lastMessagePreview: '好的，那我等你们商务同事的正式报价单。',
    createdAt: '2026-07-06T16:20:00+08:00',
    sourceType: 'private',
    groupName: null,
    groupMemberRole: 'member',
    rawMessageLinks: [],
    linkVerification: noLinks,
    extractedContacts: {
      phone: '139****8842',
      email: 'zhangjg@client-corp.cn',
      telegramHandle: null,
      wecomId: 'zhangjianguo_88',
      extractionSource: 'message_text',
    },
    friendRequestStatus: 'n/a',
    sopStage: 'chatting',
    trustScore: 95,
  },
  // ===== 群消息：好友申请已通过、可对话 =====
  {
    id: 'opp-4',
    platform: 'telegram',
    contactName: 'David Park',
    contactAvatar: '/avatars/david.png',
    summary: '群内询问初创团队方案，好友申请已通过，待开始沟通',
    matchedKeywords: ['免费试用', '基础版'],
    confidenceScore: 0.62,
    status: 'pending',
    priority: 'low',
    lastMessagePreview: 'Looking for a lightweight support tool for a small startup team.',
    createdAt: '2026-07-06T11:05:00+08:00',
    sourceType: 'group',
    groupName: 'Startup Founders Asia',
    groupMemberRole: 'member',
    rawMessageLinks: [],
    linkVerification: noLinks,
    extractedContacts: {
      phone: null,
      email: null,
      telegramHandle: '@davidpark_dev',
      wecomId: null,
      extractionSource: 'message_text',
    },
    friendRequestStatus: 'accepted',
    sopStage: 'ready_to_chat',
    trustScore: 76,
  },
  // ===== 群消息：高风险已中断（恶意链接） =====
  {
    id: 'opp-5',
    platform: 'wecom',
    contactName: '陈阿强',
    contactAvatar: '/avatars/xiaofeng.png',
    summary: '群内发布"政府补贴项目对接"信息，链接被判定为恶意站点',
    matchedKeywords: ['项目对接', '补贴'],
    confidenceScore: 0.55,
    status: 'ignored',
    priority: 'low',
    lastMessagePreview: '政府补贴项目对接，先到先得，点链接申报。',
    createdAt: '2026-07-05T19:40:00+08:00',
    sourceType: 'group',
    groupName: '企业政策申报互助群',
    groupMemberRole: 'unknown',
    rawMessageLinks: ['http://gov-subsidy-apply.top/form'],
    linkVerification: {
      status: 'malicious',
      verifiedAt: '2026-07-05T19:44:00+08:00',
      riskReasons: ['站点被多个安全引擎标记为钓鱼网站', '仿冒政务页面样式，诱导填写企业银行账户信息'],
      resolvedInfo: null,
    },
    extractedContacts: emptyContacts,
    friendRequestStatus: 'not_sent',
    sopStage: 'closed',
    trustScore: 8,
  },
  // ===== 群消息：好友申请被拒绝 =====
  {
    id: 'opp-6',
    platform: 'telegram',
    contactName: 'Emma Liu',
    contactAvatar: '/avatars/emma.png',
    summary: '连锁零售品牌群内询问多门店账号管理，好友申请被拒绝',
    matchedKeywords: ['多门店', '数据看板', '账号管理'],
    confidenceScore: 0.89,
    status: 'pending',
    priority: 'high',
    lastMessagePreview: '我们有 30 家门店，每家都需要独立账号，有推荐吗？',
    createdAt: '2026-07-06T09:30:00+08:00',
    sourceType: 'group',
    groupName: '零售数字化转型交流群',
    groupMemberRole: 'member',
    rawMessageLinks: [],
    linkVerification: noLinks,
    extractedContacts: {
      phone: null,
      email: null,
      telegramHandle: '@emmaliu_retail',
      wecomId: null,
      extractionSource: 'message_text',
    },
    friendRequestStatus: 'rejected',
    sopStage: 'friend_requested',
    trustScore: 81,
  },
]

export const mockOpportunities: Opportunity[] = seedOpportunities.map(withAgentDefaults)

// ===== 批量生成数据（用于分页演示） =====
const genNames = [
  ['林晓婷', '/avatars/lina.png'],
  ['Kevin Wu', '/avatars/michael.png'],
  ['赵敏华', '/avatars/emma.png'],
  ['Tom Zhang', '/avatars/david.png'],
  ['孙倩', '/avatars/sarah.png'],
  ['马文博', '/avatars/jianguo.png'],
  ['Alice Chen', '/avatars/lina.png'],
  ['周天佑', '/avatars/xiaofeng.png'],
] as const

const genSummaries = [
  { summary: '群内询问客服工单系统能否与钉钉审批流打通', keywords: ['工单系统', '钉钉集成'] },
  { summary: '咨询教育行业解决方案与家校沟通模块报价', keywords: ['教育行业', '报价'] },
  { summary: '询问是否支持海外服务器部署与 GDPR 合规', keywords: ['海外部署', 'GDPR'] },
  { summary: '医疗行业客户咨询患者随访自动化能力', keywords: ['医疗行业', '随访自动化'] },
  { summary: '群内征集能做小程序客服接入的服务商', keywords: ['小程序', '客服接入'] },
  { summary: '物流企业询问司机端消息推送与状态回传', keywords: ['物流', '消息推送'] },
  { summary: '想了解 AI 知识库训练需要准备哪些语料', keywords: ['AI 知识库', '语料'] },
  { summary: '连锁餐饮品牌询问会员营销自动化方案', keywords: ['会员营销', '连锁餐饮'] },
]

const genGroups = ['企业数字化服务交流群', 'SaaS 选型避坑群', '华东企服资源群', null]
const stageCycle: SopStage[] = ['detected', 'analyzing', 'verified', 'contact_extracted', 'ready_to_chat', 'chatting']

function makeGenerated(i: number): Opportunity {
  const [name, avatar] = genNames[i % genNames.length]
  const s = genSummaries[i % genSummaries.length]
  const isGroup = i % 4 !== 3
  const groupName = isGroup ? genGroups[i % 3] : null
  const hasLink = i % 5 === 2
  const stage: SopStage = isGroup ? stageCycle[i % stageCycle.length] : i % 2 === 0 ? 'chatting' : 'ready_to_chat'
  const day = 1 + (i % 6)
  const hour = 8 + (i % 12)
  const trust = 45 + ((i * 13) % 50)
  return withAgentDefaults({
    id: `opp-gen-${i + 1}`,
    platform: i % 2 === 0 ? 'telegram' : 'wecom',
    contactName: name,
    contactAvatar: avatar,
    summary: s.summary,
    matchedKeywords: s.keywords,
    confidenceScore: 0.5 + ((i * 7) % 45) / 100,
    status: i % 6 === 5 ? 'ignored' : i % 3 === 2 ? 'replied' : 'pending',
    priority: (['normal', 'high', 'low', 'urgent'] as const)[i % 4],
    lastMessagePreview: s.summary,
    createdAt: `2026-07-0${day}T${String(hour).padStart(2, '0')}:${String((i * 17) % 60).padStart(2, '0')}:00+08:00`,
    sourceType: isGroup ? 'group' : 'private',
    groupName,
    groupMemberRole: i % 7 === 0 ? 'unknown' : 'member',
    rawMessageLinks: hasLink ? [`https://vendor-info.example.com/page-${i}`] : [],
    linkVerification: hasLink
      ? {
          status: i % 10 === 7 ? 'unverified' : 'safe',
          verifiedAt: i % 10 === 7 ? null : `2026-07-0${day}T${String(hour + 1).padStart(2, '0')}:00:00+08:00`,
          riskReasons: [],
          resolvedInfo: i % 10 === 7 ? null : '落地页为正规企业官网产品介绍页，内容与消息描述一致。',
        }
      : { status: 'unverified', verifiedAt: null, riskReasons: [], resolvedInfo: null },
    extractedContacts:
      stage === 'detected' || stage === 'analyzing' || stage === 'verified'
        ? emptyContacts
        : {
            phone: i % 3 === 0 ? `13${i % 10}****${1000 + ((i * 37) % 9000)}` : null,
            email: i % 2 === 0 ? `contact${i}@company.example.com` : null,
            telegramHandle: i % 2 === 0 ? `@user_${i}_biz` : null,
            wecomId: i % 2 === 1 ? `wecom_user_${i}` : null,
            extractionSource: i % 2 === 0 ? 'message_text' : 'link_content',
          },
    friendRequestStatus: !isGroup
      ? 'n/a'
      : stage === 'ready_to_chat' || stage === 'chatting'
        ? 'accepted'
        : 'not_sent',
    sopStage: stage,
    trustScore: trust,
  })
}

for (let i = 0; i < 30; i++) {
  mockOpportunities.push(makeGenerated(i))
}

export const allKeywordTags = Array.from(new Set(mockOpportunities.flatMap((o) => o.matchedKeywords))).sort()

export const mockMessages: Record<string, ChatMessage[]> = {
  'opp-safe': [
    {
      id: 'm1',
      senderName: 'Michael Chen',
      content:
        '【采购需求】我们公司正在寻找客服自动化 API 服务商，团队约 200 人，需要与内部 CRM 打通。需求细节都在这个文档里，有意向的服务商可以联系我。https://saas-procure.example.com/rfp/2026-api',
      isFromContact: true,
      sentAt: '2026-07-07T09:32:00+08:00',
      source: null,
    },
  ],
  'opp-risk': [
    {
      id: 'm1',
      senderName: '刘晓峰',
      content: '各位老板，我手上有千万级企业订单资源要分发，点这个链接注册就能对接大客户资源，名额有限！http://biz-order-hub.xyz/reg?invite=8827',
      isFromContact: true,
      sentAt: '2026-07-07T08:58:00+08:00',
      source: null,
    },
  ],
  'opp-1': [
    {
      id: 'm1',
      senderName: '王丽娜',
      content: '您好，我是华信金融的采购负责人。我们对你们的产品比较感兴趣，但金融行业对数据安全要求很高。',
      isFromContact: true,
      sentAt: '2026-07-07T08:40:00+08:00',
      source: null,
    },
    {
      id: 'm2',
      senderName: '王丽娜',
      content: '你们有等保三级认证吗？私有化部署大概什么周期？',
      isFromContact: true,
      sentAt: '2026-07-07T08:45:00+08:00',
      source: null,
    },
  ],
  'opp-2': [
    {
      id: 'm1',
      senderName: 'Sarah Kim',
      content: 'Hi all, we run a cross-border e-commerce store. Any vendors here support Korean and Japanese customer replies?',
      isFromContact: true,
      sentAt: '2026-07-06T23:12:00+08:00',
      source: null,
    },
  ],
  'opp-3': [
    {
      id: 'm1',
      senderName: '张建国',
      content: '我们的年度合同这个月底到期，想聊聊续约的事。另外业务扩张了，需要再加 50 个坐席。',
      isFromContact: true,
      sentAt: '2026-07-06T16:02:00+08:00',
      source: null,
    },
    {
      id: 'm2',
      senderName: '商机助手',
      content: '张总您好！感谢一年来的信任。续约加增购 50 坐席可以叠加老客户专属折扣，我让商务同事今天整理一份正式报价单发给您。',
      isFromContact: false,
      sentAt: '2026-07-06T16:15:00+08:00',
      source: 'human',
    },
    {
      id: 'm3',
      senderName: '张建国',
      content: '好的，那我等你们商务同事的正式报价单。',
      isFromContact: true,
      sentAt: '2026-07-06T16:20:00+08:00',
      source: null,
    },
  ],
  'opp-4': [
    {
      id: 'm1',
      senderName: 'David Park',
      content: 'Looking for a lightweight support tool for a small startup team. Any recommendations?',
      isFromContact: true,
      sentAt: '2026-07-06T11:05:00+08:00',
      source: null,
    },
  ],
  'opp-5': [
    {
      id: 'm1',
      senderName: '陈阿强',
      content: '政府补贴项目对接，先到先得，点链接申报。http://gov-subsidy-apply.top/form',
      isFromContact: true,
      sentAt: '2026-07-05T19:40:00+08:00',
      source: null,
    },
  ],
  'opp-6': [
    {
      id: 'm1',
      senderName: 'Emma Liu',
      content: '我们有 30 家门店，每家都需要独立账号，有推荐吗？',
      isFromContact: true,
      sentAt: '2026-07-06T09:30:00+08:00',
      source: null,
    },
  ],
}

export const mockTemplates: ReplyTemplate[] = [
  {
    id: 'tpl-1',
    title: '首次咨询欢迎语',
    content: '您好 {{联系人姓名}}！感谢您的关注。我是您的专属顾问，请问您目前主要想解决什么业务问题？我可以为您做针对性介绍。',
    category: '开场白',
  },
  {
    id: 'tpl-9',
    title: '好友申请打招呼语',
    content: '您好 {{联系人姓名}}，我在「{{群名称}}」看到您发布的需求，我们正好提供相关解决方案，希望能加个好友详细沟通，不会频繁打扰您。',
    category: '开场白',
  },
  {
    id: 'tpl-2',
    title: '企业版功能介绍',
    content: '{{联系人姓名}} 您好，企业版包含 API 接入、SSO 单点登录、专属客户成功经理与 SLA 保障，适合 {{团队规模}} 以上团队。需要我发一份详细功能对比表吗？',
    category: '产品介绍',
  },
  {
    id: 'tpl-3',
    title: '报价跟进',
    content: '{{联系人姓名}} 您好，报价单已为您整理好。基于 {{团队规模}} 的规模，我们提供了阶梯折扣方案，有效期至本月底。方便的话我们可以约 15 分钟电话沟通细节。',
    category: '报价与商务',
  },
  {
    id: 'tpl-4',
    title: '批量采购折扣说明',
    content: '关于批量采购：50 席以上享 9 折，100 席以上享 8.5 折，200 席以上可申请专属企业价。{{联系人姓名}}，您预计的采购规模是多少呢？',
    category: '报价与商务',
  },
  {
    id: 'tpl-5',
    title: '私有化部署说明',
    content: '我们支持私有化部署，已通过等保三级与 ISO 27001 认证。标准交付周期为 4-6 周，包含部署实施与团队培训。可以为 {{公司名称}} 安排一次技术评估会议。',
    category: '技术与安全',
  },
  {
    id: 'tpl-6',
    title: '试用引导',
    content: '{{联系人姓名}} 您好，我们提供 14 天全功能免费试用，无需绑定支付方式。我可以帮您开通企业试用账户，并安排 30 分钟的上手指导，您看什么时间方便？',
    category: '试用与转化',
  },
  {
    id: 'tpl-7',
    title: '非工作时间自动回复',
    content: '您好 {{联系人姓名}}，感谢您的消息！现在是非工作时间，我已记录您的需求，工作时间（{{工作时间}}）内会第一时间给您详细答复。如有紧急事项请留言说明。',
    category: '自动回复',
  },
  {
    id: 'tpl-8',
    title: '续约优惠通知',
    content: '{{联系人姓名}} 您好，您的年度合约即将到期。作为老客户，续约可享专属折扣，增购坐席同样适用。需要我为您出一份续约方案吗？',
    category: '报价与商务',
  },
]

export const templateCategories = ['全部', '开场白', '产品介绍', '报价与商务', '技术与安全', '试用与转化', '自动回复']

export const incomingOpportunity: Opportunity = withAgentDefaults({
  id: 'opp-new',
  platform: 'telegram',
  contactName: 'Emma Liu',
  contactAvatar: '/avatars/emma.png',
  summary: '群内新消息：连锁零售品牌询问多门店账号管理与数据看板功能',
  matchedKeywords: ['多门店', '数据看板', '账号管理'],
  confidenceScore: 0.89,
  status: 'pending',
  priority: 'high',
  lastMessagePreview: '我们有 30 家门店，每家都需要独立账号，你们支持吗？',
  createdAt: '2026-07-07T10:02:00+08:00',
  sourceType: 'group',
  groupName: '零售数字化转型交流群',
  groupMemberRole: 'member',
  rawMessageLinks: [],
  linkVerification: noLinks,
  extractedContacts: emptyContacts,
  friendRequestStatus: 'not_sent',
  sopStage: 'detected',
  trustScore: 70,
})
