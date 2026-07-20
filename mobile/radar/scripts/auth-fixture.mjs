import { createServer } from 'node:http';
import { createHash, randomBytes, randomUUID } from 'node:crypto';

const host = '127.0.0.1';
const port = 8788;
const accessToken = 'fixture-access-token-0123456789';
const user = Object.freeze({
  id: '01234567-89ab-cdef-0123-456789abcdef',
  email: 'developer@example.test',
  displayName: 'Fixture Developer',
  avatarUrl: '',
  isAdmin: false,
  hasPassword: true,
});

// Ephemeral development-only device state. Production persists hashes in PostgreSQL;
// this fixture intentionally resets everything whenever the process restarts.
const fixtureDevices = new Map();
const fixtureDeviceIdByInstallationHash = new Map();
const fixtureCredentials = new Map();
const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

function sha256(value) {
  return createHash('sha256').update(value).digest('hex');
}

function newDeviceRefreshToken() {
  return `radar_device_1_${randomBytes(32).toString('base64url')}`;
}

function deviceRead(device) {
  return {
    id: device.id,
    platform: device.platform,
    status: device.status,
    displayName: device.displayName,
    appVariant: device.appVariant,
    appVersion: device.appVersion,
    appBuild: device.appBuild,
    osVersion: device.osVersion,
    locale: device.locale,
    timezone: device.timezone,
    capabilities: device.capabilities,
    lastSeenAt: device.lastSeenAt,
    revokedAt: device.revokedAt,
    createdAt: device.createdAt,
    updatedAt: device.updatedAt,
  };
}

function issueDeviceSession(device) {
  const refreshToken = newDeviceRefreshToken();
  const expiresAt = new Date(Date.now() + 30 * 24 * 60 * 60 * 1_000).toISOString();
  fixtureCredentials.set(sha256(refreshToken), {
    deviceId: device.id,
    expiresAt,
    status: 'active',
  });
  return {
    accessToken,
    tokenType: 'bearer',
    deviceRefreshToken: refreshToken,
    deviceRefreshTokenExpiresAt: expiresAt,
    device: deviceRead(device),
    user,
  };
}

function revokeFixtureDevice(device, reason = 'fixture_revoked') {
  const now = new Date().toISOString();
  device.status = 'revoked';
  device.revokedAt = now;
  device.updatedAt = now;
  device.revocationReason = reason;
  for (const credential of fixtureCredentials.values()) {
    if (credential.deviceId === device.id) credential.status = 'revoked';
  }
}

function validDeviceRegistration(body) {
  return body &&
    uuidPattern.test(body.installationId ?? '') &&
    (body.platform === 'ios' || body.platform === 'android') &&
    typeof body.displayName === 'string' && body.displayName.length <= 100 &&
    (body.appVariant === 'development' || body.appVariant === 'production') &&
    typeof body.appVersion === 'string' && /^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(body.appVersion) &&
    typeof body.appBuild === 'string' && /^[0-9A-Za-z._+-]{1,32}$/.test(body.appBuild) &&
    (!body.capabilities || (typeof body.capabilities === 'object' && !Array.isArray(body.capabilities)));
}

function registerFixtureDevice(body) {
  const now = new Date().toISOString();
  const installationHash = sha256(body.installationId);
  const existingId = fixtureDeviceIdByInstallationHash.get(installationHash);
  let device = existingId ? fixtureDevices.get(existingId) : null;
  if (device?.status === 'revoked') return null;

  if (!device) {
    device = {
      id: randomUUID(),
      status: 'active',
      revokedAt: null,
      createdAt: now,
    };
    fixtureDevices.set(device.id, device);
    fixtureDeviceIdByInstallationHash.set(installationHash, device.id);
  } else {
    for (const credential of fixtureCredentials.values()) {
      if (credential.deviceId === device.id && credential.status === 'active') {
        credential.status = 'rotated';
      }
    }
  }

  Object.assign(device, {
    platform: body.platform,
    displayName: body.displayName,
    appVariant: body.appVariant,
    appVersion: body.appVersion,
    appBuild: body.appBuild,
    osVersion: body.osVersion ?? null,
    locale: body.locale ?? null,
    timezone: body.timezone ?? null,
    capabilities: body.capabilities ?? {},
    lastSeenAt: now,
    updatedAt: now,
  });
  return issueDeviceSession(device);
}

function rotateFixtureCredential(request) {
  const authorization = request.headers.authorization ?? '';
  const match = /^Bearer (radar_device_1_[A-Za-z0-9_-]{43})$/.exec(authorization);
  if (!match) return null;
  const credential = fixtureCredentials.get(sha256(match[1]));
  const device = credential ? fixtureDevices.get(credential.deviceId) : null;
  if (!credential || !device || device.status !== 'active') return null;
  if (credential.status === 'rotated') {
    revokeFixtureDevice(device, 'credential_reuse_detected');
    return null;
  }
  if (credential.status !== 'active' || Date.parse(credential.expiresAt) <= Date.now()) return null;
  credential.status = 'rotated';
  const now = new Date().toISOString();
  device.lastSeenAt = now;
  device.updatedAt = now;
  return issueDeviceSession(device);
}

function opportunity(overrides) {
  return Object.freeze({
    id: '11111111-1111-4111-8111-111111111111',
    opportunityType: 'business',
    platform: 'telegram',
    contactName: '星海科技采购群',
    contactAvatar: '',
    summary: '计划为多个团队采购年度协作方案，希望本周确认报价。',
    matchedKeywords: ['报价', '采购'],
    confidenceScore: 0.94,
    status: 'pending',
    internalStatus: 'pending_human',
    priority: 'urgent',
    lastMessagePreview: '预算已经批准，能否发一份 80 人的正式报价？',
    createdAt: '2026-07-17T01:00:00Z',
    updatedAt: '2026-07-17T01:05:00Z',
    sourceType: 'group',
    groupName: '星海科技采购群',
    groupMemberRole: 'member',
    rawMessageLinks: [],
    linkVerification: {
      status: 'safe',
      verifiedAt: '2026-07-17T01:04:00Z',
      riskReasons: [],
      resolvedInfo: '企业官网与采购需求一致',
    },
    extractedContacts: {
      phone: null,
      email: 'buyer@example.test',
      telegramHandle: null,
      wecomId: null,
      extractionSource: 'message_text',
    },
    friendRequestStatus: 'not_sent',
    sopStage: 'verified',
    trustScore: 91,
    agentActions: [],
    agentAnalysisStatus: 'completed',
    agentAnalysisError: null,
    agentAnalyzedAt: '2026-07-17T01:04:00Z',
    attentionRequired: true,
    archivedAt: null,
    archivedByUserId: null,
    archiveReason: null,
    ...overrides,
  });
}

const opportunities = Object.freeze([
  opportunity({
    agentActions: [{
      actionType: 'notify_user',
      reason: '采购预算已获批，建议优先人工跟进。',
      target: null,
      draft: null,
      requiresApproval: true,
    }],
  }),
  opportunity({
    id: '22222222-2222-4222-8222-222222222222',
    platform: 'wecom',
    contactName: '林经理',
    summary: '已收到产品演示，希望进一步讨论企业微信接入。',
    matchedKeywords: ['企业微信', '演示'],
    confidenceScore: 0.82,
    status: 'replied',
    internalStatus: 'following',
    priority: 'high',
    lastMessagePreview: '演示很清楚，我们内部评估后再联系。',
    createdAt: '2026-07-16T06:20:00Z',
    updatedAt: '2026-07-16T07:00:00Z',
    sourceType: 'private',
    groupName: null,
    friendRequestStatus: 'n/a',
    sopStage: 'contact_extracted',
    trustScore: 68,
    attentionRequired: false,
    agentActions: [],
  }),
  opportunity({
    id: '33333333-3333-4333-8333-333333333333',
    contactName: '产品交流频道',
    summary: '泛化产品咨询，当前没有明确采购计划。',
    matchedKeywords: ['产品咨询'],
    confidenceScore: 0.51,
    status: 'ignored',
    internalStatus: 'ignored',
    priority: 'normal',
    lastMessagePreview: '先了解一下功能，暂时没有预算。',
    createdAt: '2026-07-14T03:10:00Z',
    updatedAt: '2026-07-14T03:12:00Z',
    sourceType: 'group',
    groupName: '产品交流频道',
    sopStage: 'detected',
    trustScore: 35,
    agentAnalysisStatus: 'not_requested',
    agentAnalyzedAt: null,
    attentionRequired: false,
    agentActions: [],
    linkVerification: {},
    extractedContacts: {},
  }),
]);

const opportunityDetails = Object.freeze({
  [opportunities[0].id]: Object.freeze({
    ...opportunities[0],
    detectionReason: '命中采购与报价规则，并由 pi Agent 完成结构化复核。',
    aiReplyDraft: '可以，我们会在人工确认后发送 80 人企业方案报价。',
    finalReply: null,
  }),
  [opportunities[1].id]: Object.freeze({
    ...opportunities[1],
    detectionReason: '命中企业微信接入与演示关键词。',
    aiReplyDraft: null,
    finalReply: '感谢反馈，随时可以继续沟通接入方案。',
  }),
  [opportunities[2].id]: Object.freeze({
    ...opportunities[2],
    detectionReason: null,
    aiReplyDraft: null,
    finalReply: null,
  }),
});

const detailOverrides = new Map();
const manualReplyResults = new Map();
const tailMessagesByOpportunity = new Map();
const replyTemplates = Object.freeze([
  Object.freeze({
    id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
    title: '报价确认',
    content: '收到，我们会在人工确认后发送正式报价与实施说明。',
    category: '销售跟进',
  }),
  Object.freeze({
    id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
    title: '预约演示',
    content: '可以，请告诉我方便的时间，我们会安排产品演示。',
    category: '产品演示',
  }),
]);

let settingsBundle = {
  detection: { keywords: ['报价', '采购'], aiSemanticsEnabled: true },
  workSchedule: {
    timezone: 'Asia/Shanghai',
    slots: [
      { weekday: 1, start: '09:00', end: '12:00' },
      { weekday: 1, start: '13:00', end: '18:00' },
      { weekday: 2, start: '09:00', end: '18:00' },
    ],
    autoReplyOutsideHours: true,
    isDefault: false,
  },
  notifications: {
    newOpportunityEnabled: true,
    aiRepliedEnabled: true,
    dailyDigestEnabled: false,
    urgentOnly: false,
  },
  capabilities: { pushAvailable: false, wecomUserBindingAvailable: true },
};

const telegramHealth = Object.freeze({
  mode: 'live',
  botConfigured: true,
  botUsername: 'fixture_radar_bot',
  businessAvailable: false,
  mtprotoQrAvailable: false,
  listenerMode: 'vps-long-running',
  legacyMonitoringActive: false,
  legacyActiveSourceCount: 0,
  message: '普通账号 QR 尚未由管理员配置；不会收集用户 API Hash、手机号、验证码或 Session。',
});

let telegramConnections = [{
  id: '44444444-4444-4444-8444-444444444444',
  connectionType: 'bot_chat',
  status: 'connected',
  enabled: true,
  label: 'Fixture 采购群 Bot',
  capabilities: { receive_group_messages: true, can_reply: true },
  lastError: null,
  lastCheckedAt: '2026-07-17T02:00:00Z',
  updatedAt: '2026-07-17T02:00:00Z',
  sources: [
    {
      id: '55555555-5555-4555-8555-555555555555',
      connectionId: '44444444-4444-4444-8444-444444444444',
      sourceType: 'group',
      externalChatId: '-100100',
      displayName: '星海科技采购群',
      username: 'fixture_buyers',
      enabled: true,
      quotaPaused: false,
      quotaReason: null,
      lastError: null,
      updatedAt: '2026-07-17T02:00:00Z',
    },
    {
      id: '66666666-6666-4666-8666-666666666666',
      connectionId: '44444444-4444-4444-8444-444444444444',
      sourceType: 'channel',
      externalChatId: '-100200',
      displayName: '行业采购频道',
      username: null,
      enabled: false,
      quotaPaused: true,
      quotaReason: '当前套餐额度已满；升级或移除其他来源后可恢复。',
      lastError: null,
      updatedAt: '2026-07-17T02:00:00Z',
    },
  ],
}];

const planEntitlements = Object.freeze({
  free: Object.freeze({
    planCode: 'free',
    telegramGroupLimit: 1,
    wecomGroupLimit: 1,
    combinedGroupLimit: 2,
    piAgentAnalysisMonthlyLimit: 100,
  }),
  plus: Object.freeze({
    planCode: 'plus',
    telegramGroupLimit: null,
    wecomGroupLimit: null,
    combinedGroupLimit: 10,
    piAgentAnalysisMonthlyLimit: 1_000,
  }),
  pro: Object.freeze({
    planCode: 'pro',
    telegramGroupLimit: null,
    wecomGroupLimit: null,
    combinedGroupLimit: 50,
    piAgentAnalysisMonthlyLimit: 5_000,
  }),
  max: Object.freeze({
    planCode: 'max',
    telegramGroupLimit: null,
    wecomGroupLimit: null,
    combinedGroupLimit: 100,
    piAgentAnalysisMonthlyLimit: 10_000,
  }),
});

const subscriptionCatalog = Object.freeze(
  ['free', 'plus', 'pro', 'max'].map((planCode, rank) => Object.freeze({
    planCode,
    displayName: planCode === 'free'
      ? 'Free'
      : planCode[0].toUpperCase() + planCode.slice(1),
    rank,
    entitlements: planEntitlements[planCode],
    availableIntervals: planCode === 'free' ? [] : ['monthly', 'annual'],
    revenuecatPackageIdentifiers: planCode === 'free'
      ? []
      : [`${planCode}_monthly`, `${planCode}_annual`],
  })),
);

const subscriptionUsage = Object.freeze({
  planCode: 'free',
  subscriptionStatus: 'inactive',
  periodStart: '2026-07-01T00:00:00Z',
  periodEnd: '2026-08-01T00:00:00Z',
  cancelAtPeriodEnd: false,
  entitlements: planEntitlements.free,
  telegramGroupsUsed: 1,
  wecomGroupsUsed: 0,
  combinedGroupsUsed: 1,
  aiAnalysesConsumed: 42,
  aiAnalysesReserved: 3,
  aiAnalysesRemaining: 55,
  effectiveStore: null,
  billingInterval: null,
  billingPeriodStart: null,
  billingPeriodEnd: null,
  usagePeriodStart: '2026-07-01T00:00:00Z',
  usagePeriodEnd: '2026-08-01T00:00:00Z',
  entitlementExpiresAt: null,
  willRenew: false,
  billingIssue: false,
  multipleActiveSubscriptions: false,
  managementUrl: null,
  lastSyncedAt: null,
});

const subscriptionManagement = Object.freeze({
  store: null,
  managementUrl: null,
  instruction: 'No paid subscription is currently active.',
  canOpenInCurrentClient: false,
});

function fixtureMessage(item, index) {
  const incoming = index % 2 === 0;
  return Object.freeze({
    id: `${item.id.slice(0, 8)}-4444-4444-8444-${String(index + 10).padStart(12, '0')}`,
    senderName: incoming ? item.contactName : '商机助手',
    content: incoming
      ? `${item.contactName}的第 ${index + 1} 条需求上下文。`
      : `第 ${index + 1} 条已审核跟进记录。`,
    isFromContact: incoming,
    sentAt: new Date(Date.UTC(2026, 6, 17, 1, index)).toISOString(),
    source: incoming ? 'human' : index % 4 === 1 ? 'ai' : 'human',
  });
}

const messagesByOpportunity = Object.freeze({
  [opportunities[0].id]: Object.freeze(Array.from(
    { length: 55 },
    (_, index) => fixtureMessage(opportunities[0], index),
  )),
  [opportunities[1].id]: Object.freeze([]),
  [opportunities[2].id]: Object.freeze([
    fixtureMessage(opportunities[2], 0),
  ]),
});

function allMessages(opportunityId) {
  return [
    ...(messagesByOpportunity[opportunityId] ?? []),
    ...(tailMessagesByOpportunity.get(opportunityId) ?? []),
  ];
}

const trustRanges = {
  trusted: [80, 100],
  unverified: [60, 79],
  suspicious: [40, 59],
  risky: [0, 39],
};

function authorize(request, response) {
  if (request.headers.authorization === `Bearer ${accessToken}`) return true;
  send(response, 401, { detail: '登录已过期' });
  return false;
}

function dashboardResponse(url) {
  const status = url.searchParams.get('status');
  const platform = url.searchParams.get('platform');
  const sourceType = url.searchParams.get('source_type');
  const createdFrom = url.searchParams.get('created_from');
  const createdTo = url.searchParams.get('created_to');
  const trustLevels = url.searchParams.getAll('trust_levels');
  const sopStages = url.searchParams.getAll('sop_stages');
  const keywords = url.searchParams.getAll('keywords');
  const sort = url.searchParams.get('sort') ?? 'newest';
  const limit = Number(url.searchParams.get('limit') ?? 20);
  const offset = Number(url.searchParams.get('offset') ?? 0);

  const filtered = opportunities.filter((item) => {
    if (status && item.status !== status) return false;
    if (platform && item.platform !== platform) return false;
    if (sourceType && item.sourceType !== sourceType) return false;
    if (createdFrom && Date.parse(item.createdAt) < Date.parse(createdFrom)) return false;
    if (createdTo && Date.parse(item.createdAt) > Date.parse(createdTo)) return false;
    if (
      trustLevels.length > 0 &&
      !trustLevels.some((level) => {
        const range = trustRanges[level];
        return range && item.trustScore >= range[0] && item.trustScore <= range[1];
      })
    ) return false;
    if (sopStages.length > 0 && !sopStages.includes(item.sopStage)) return false;
    if (keywords.length > 0 && !keywords.some((keyword) => item.matchedKeywords.includes(keyword))) return false;
    return true;
  });
  filtered.sort((left, right) => {
    if (sort === 'oldest') return Date.parse(left.createdAt) - Date.parse(right.createdAt);
    if (sort === 'confidence') return right.confidenceScore - left.confidenceScore;
    if (sort === 'trust') return right.trustScore - left.trustScore;
    return Date.parse(right.createdAt) - Date.parse(left.createdAt);
  });

  return {
    items: filtered.slice(offset, offset + limit),
    total: filtered.length,
    limit,
    offset,
    pendingCount: opportunities.filter((item) => item.status === 'pending').length,
    attentionItems: opportunities.filter((item) => item.status === 'pending' && item.attentionRequired),
    keywordOptions: Array.from(new Set(opportunities.flatMap((item) => item.matchedKeywords))).sort(),
  };
}

function opportunityDetail(opportunityId) {
  const detail = opportunityDetails[opportunityId];
  if (!detail) return null;
  return { ...detail, ...(detailOverrides.get(opportunityId) ?? {}) };
}

function updateOpportunityDetail(opportunityId, overrides) {
  const detail = opportunityDetail(opportunityId);
  if (!detail) return null;
  const next = {
    ...detail,
    ...overrides,
    updatedAt: new Date().toISOString(),
  };
  detailOverrides.set(opportunityId, next);
  return next;
}

function publicStatus(internalStatus) {
  if (internalStatus === 'ignored') return 'ignored';
  if (internalStatus === 'pending_human' || internalStatus === 'ai_auto_reply') return 'pending';
  return 'replied';
}

function messagePage(url) {
  const opportunityId = url.searchParams.get('opportunity_id');
  const limit = Number(url.searchParams.get('limit') ?? 50);
  const offset = Number(url.searchParams.get('offset') ?? 0);
  if (
    !opportunityId ||
    !Number.isInteger(limit) || limit < 1 || limit > 200 ||
    !Number.isInteger(offset) || offset < 0
  ) return null;
  const items = allMessages(opportunityId);
  return {
    items: items.slice(offset, offset + limit),
    total: items.length,
    limit,
    offset,
  };
}

function send(response, status, body) {
  response.writeHead(status, {
    'Cache-Control': 'no-store',
    'Content-Type': 'application/json; charset=utf-8',
    'X-Request-Id': 'local-auth-fixture',
  });
  response.end(JSON.stringify(body));
}

async function readJson(request) {
  const chunks = [];
  let size = 0;
  for await (const chunk of request) {
    size += chunk.length;
    if (size > 16_384) throw new Error('request-too-large');
    chunks.push(chunk);
  }
  return JSON.parse(Buffer.concat(chunks).toString('utf8'));
}

const server = createServer(async (request, response) => {
  try {
    if (request.method === 'POST' && request.url === '/api/v1/auth/password/login') {
      const body = await readJson(request);
      if (body.email !== user.email || body.password !== 'fixture-password') {
        send(response, 401, { detail: '邮箱或密码错误' });
        return;
      }
      send(response, 200, { accessToken, tokenType: 'bearer', user });
      return;
    }

    if (request.method === 'GET' && request.url === '/api/v1/auth/me') {
      if (!authorize(request, response)) return;
      send(response, 200, user);
      return;
    }

    const url = new URL(request.url ?? '/', `http://${host}:${port}`);
    if (request.method === 'POST' && url.pathname === '/api/v1/devices/register') {
      if (!authorize(request, response)) return;
      const body = await readJson(request);
      if (!validDeviceRegistration(body)) {
        send(response, 422, { detail: 'invalid device registration' });
        return;
      }
      const session = registerFixtureDevice(body);
      send(
        response,
        session ? 201 : 409,
        session ?? { detail: 'device installation is revoked' },
      );
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/devices') {
      if (!authorize(request, response)) return;
      const devices = Array.from(fixtureDevices.values())
        .sort((left, right) => Date.parse(right.lastSeenAt) - Date.parse(left.lastSeenAt))
        .map(deviceRead);
      send(response, 200, devices);
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/devices/current/capabilities') {
      if (!authorize(request, response)) return;
      send(response, 200, {
        agentToolsAvailable: false,
        deviceAgentAvailable: false,
        e2eeAvailable: false,
        hostedFallbackAvailable: false,
        pushAvailable: false,
        rnClientSupported: true,
        syncAvailable: false,
      });
      return;
    }

    if (request.method === 'POST' && url.pathname === '/api/v1/devices/credentials/rotate') {
      const session = rotateFixtureCredential(request);
      send(response, session ? 200 : 401, session ?? { detail: 'invalid device credential' });
      return;
    }

    const deviceRevokeMatch = /^\/api\/v1\/devices\/([0-9a-fA-F-]+)\/revoke$/.exec(url.pathname);
    if (request.method === 'POST' && deviceRevokeMatch) {
      if (!authorize(request, response)) return;
      const device = fixtureDevices.get(deviceRevokeMatch[1]);
      if (!device) {
        send(response, 404, { detail: 'device not found' });
        return;
      }
      if (device.status !== 'revoked') revokeFixtureDevice(device, 'user_revoked');
      send(response, 200, deviceRead(device));
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/opportunities/dashboard') {
      if (!authorize(request, response)) return;
      send(response, 200, dashboardResponse(url));
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/templates') {
      if (!authorize(request, response)) return;
      send(response, 200, replyTemplates);
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/settings/me') {
      if (!authorize(request, response)) return;
      send(response, 200, settingsBundle);
      return;
    }

    if (request.method === 'PATCH' && url.pathname === '/api/v1/settings/detection') {
      if (!authorize(request, response)) return;
      const body = await readJson(request);
      if (!Array.isArray(body.keywords) || typeof body.aiSemanticsEnabled !== 'boolean') {
        send(response, 422, { detail: 'invalid detection settings' });
        return;
      }
      const keywords = [];
      const seen = new Set();
      for (const value of body.keywords) {
        if (typeof value !== 'string') {
          send(response, 422, { detail: 'invalid detection keyword' });
          return;
        }
        const keyword = value.trim();
        if (!keyword) continue;
        if (keyword.length > 64 || keywords.length >= 200) {
          send(response, 422, { detail: 'invalid detection keyword' });
          return;
        }
        const key = keyword.toLocaleLowerCase();
        if (!seen.has(key)) {
          seen.add(key);
          keywords.push(keyword);
        }
      }
      settingsBundle = {
        ...settingsBundle,
        detection: { keywords, aiSemanticsEnabled: body.aiSemanticsEnabled },
      };
      send(response, 200, settingsBundle.detection);
      return;
    }

    if (request.method === 'PATCH' && url.pathname === '/api/v1/settings/work-schedule') {
      if (!authorize(request, response)) return;
      const body = await readJson(request);
      const validSlots = Array.isArray(body.slots) && body.slots.length <= 168 && body.slots.every((slot) =>
        Number.isInteger(slot.weekday) && slot.weekday >= 1 && slot.weekday <= 7 &&
        /^([01]\d|2[0-3]):[0-5]\d$/.test(slot.start) &&
        /^([01]\d|2[0-3]):[0-5]\d$/.test(slot.end) && slot.start < slot.end);
      let validTimezone = typeof body.timezone === 'string' && body.timezone.length <= 64;
      try {
        new Intl.DateTimeFormat('en-US', { timeZone: body.timezone }).format();
      } catch {
        validTimezone = false;
      }
      if (!validSlots || !validTimezone || typeof body.autoReplyOutsideHours !== 'boolean') {
        send(response, 422, { detail: 'invalid work schedule' });
        return;
      }
      settingsBundle = {
        ...settingsBundle,
        workSchedule: { ...body, isDefault: false },
      };
      send(response, 200, settingsBundle.workSchedule);
      return;
    }

    if (request.method === 'PATCH' && url.pathname === '/api/v1/settings/notifications') {
      if (!authorize(request, response)) return;
      const body = await readJson(request);
      const keys = ['newOpportunityEnabled', 'aiRepliedEnabled', 'dailyDigestEnabled', 'urgentOnly'];
      if (!keys.every((key) => typeof body[key] === 'boolean')) {
        send(response, 422, { detail: 'invalid notification settings' });
        return;
      }
      settingsBundle = {
        ...settingsBundle,
        notifications: Object.fromEntries(keys.map((key) => [key, body[key]])),
      };
      send(response, 200, settingsBundle.notifications);
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/integrations/telegram/health') {
      if (!authorize(request, response)) return;
      send(response, 200, telegramHealth);
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/integrations/telegram/connections') {
      if (!authorize(request, response)) return;
      send(response, 200, telegramConnections);
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/subscriptions/catalog') {
      if (!authorize(request, response)) return;
      send(response, 200, subscriptionCatalog);
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/subscriptions/me') {
      if (!authorize(request, response)) return;
      send(response, 200, subscriptionUsage);
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/subscriptions/management') {
      if (!authorize(request, response)) return;
      send(response, 200, subscriptionManagement);
      return;
    }

    if (request.method === 'POST' && url.pathname === '/api/v1/subscriptions/sync') {
      if (!authorize(request, response)) return;
      send(response, 503, { detail: 'Payments are not configured in the local fixture' });
      return;
    }

    const telegramConnectionMatch = /^\/api\/v1\/integrations\/telegram\/connections\/([0-9a-fA-F-]+)$/.exec(url.pathname);
    if (request.method === 'PATCH' && telegramConnectionMatch) {
      if (!authorize(request, response)) return;
      const body = await readJson(request);
      const index = telegramConnections.findIndex((item) => item.id === telegramConnectionMatch[1]);
      if (index < 0) {
        send(response, 404, { detail: 'connection not found' });
        return;
      }
      if (typeof body.enabled !== 'boolean') {
        send(response, 422, { detail: 'invalid connection update' });
        return;
      }
      const current = telegramConnections[index];
      const updated = {
        ...current,
        enabled: body.enabled,
        status: body.enabled ? 'connected' : 'disabled',
        updatedAt: new Date().toISOString(),
      };
      telegramConnections = telegramConnections.map((item) => item.id === updated.id ? updated : item);
      send(response, 200, updated);
      return;
    }

    const aiDraftMatch = /^\/api\/v1\/opportunities\/([0-9a-fA-F-]+)\/ai-draft$/.exec(url.pathname);
    if (request.method === 'POST' && aiDraftMatch) {
      if (!authorize(request, response)) return;
      const detail = opportunityDetail(aiDraftMatch[1]);
      if (!detail) {
        send(response, 404, { detail: 'opportunity not found' });
        return;
      }
      send(response, 200, {
        opportunity_id: detail.id,
        draft: '可以，我们会在人工确认后发送 80 人企业方案报价与实施说明。',
      });
      return;
    }

    const claimMatch = /^\/api\/v1\/opportunities\/([0-9a-fA-F-]+)\/claim$/.exec(url.pathname);
    if (request.method === 'POST' && claimMatch) {
      if (!authorize(request, response)) return;
      const detail = updateOpportunityDetail(claimMatch[1], { assignedTo: user.id });
      send(response, detail ? 200 : 404, detail ?? { detail: 'opportunity not found' });
      return;
    }

    const statusMatch = /^\/api\/v1\/opportunities\/([0-9a-fA-F-]+)\/status$/.exec(url.pathname);
    if (request.method === 'PATCH' && statusMatch) {
      if (!authorize(request, response)) return;
      const body = await readJson(request);
      const allowedStatuses = new Set([
        'pending_human',
        'ai_auto_reply',
        'replied',
        'following',
        'ignored',
        'closed',
      ]);
      if (!allowedStatuses.has(body.status)) {
        send(response, 422, { detail: 'invalid opportunity status' });
        return;
      }
      const detail = updateOpportunityDetail(statusMatch[1], {
        internalStatus: body.status,
        status: publicStatus(body.status),
      });
      send(response, detail ? 200 : 404, detail ?? { detail: 'opportunity not found' });
      return;
    }

    const manualReplyMatch = /^\/api\/v1\/opportunities\/([0-9a-fA-F-]+)\/manual-reply\/result$/.exec(url.pathname);
    if (request.method === 'POST' && manualReplyMatch) {
      if (!authorize(request, response)) return;
      const detail = opportunityDetail(manualReplyMatch[1]);
      if (!detail) {
        send(response, 404, { detail: 'opportunity not found' });
        return;
      }
      const idempotencyKey = request.headers['idempotency-key'];
      const body = await readJson(request);
      const text = typeof body.text === 'string' ? body.text.trim() : '';
      if (
        typeof idempotencyKey !== 'string' ||
        !/^[0-9a-fA-F-]{36}$/.test(idempotencyKey) ||
        text.length < 1 ||
        text.length > 4000
      ) {
        send(response, 422, { detail: 'invalid manual reply request' });
        return;
      }
      const fingerprint = JSON.stringify({ opportunityId: detail.id, text });
      const existing = manualReplyResults.get(idempotencyKey);
      if (existing) {
        send(response, existing.fingerprint === fingerprint ? 200 : 409,
          existing.fingerprint === fingerprint ? existing.result : { detail: 'idempotency key conflict' });
        return;
      }
      const sentAt = new Date().toISOString();
      const updated = updateOpportunityDetail(detail.id, {
        finalReply: text,
        internalStatus: 'following',
        status: 'replied',
      });
      const message = {
        id: idempotencyKey,
        senderName: '商机助手',
        content: text,
        isFromContact: false,
        sentAt,
        source: 'human',
      };
      const tail = tailMessagesByOpportunity.get(detail.id) ?? [];
      tailMessagesByOpportunity.set(detail.id, [...tail, message]);
      const result = {
        opportunity: updated,
        message,
        messageTotal: allMessages(detail.id).length,
      };
      manualReplyResults.set(idempotencyKey, { fingerprint, result });
      send(response, 200, result);
      return;
    }

    const detailMatch = /^\/api\/v1\/opportunities\/([0-9a-fA-F-]+)$/.exec(url.pathname);
    if (request.method === 'GET' && detailMatch) {
      if (!authorize(request, response)) return;
      const detail = opportunityDetail(detailMatch[1]);
      send(response, detail ? 200 : 404, detail ?? { detail: 'opportunity not found' });
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/messages/page') {
      if (!authorize(request, response)) return;
      const page = messagePage(url);
      send(response, page ? 200 : 422, page ?? { detail: 'invalid message page' });
      return;
    }

    if (request.method === 'GET' && url.pathname === '/api/v1/messages') {
      if (!authorize(request, response)) return;
      const opportunityId = url.searchParams.get('opportunity_id');
      send(response, 200, allMessages(opportunityId).slice(-500));
      return;
    }

    send(response, 404, { detail: 'fixture route not found' });
  } catch (error) {
    send(response, error instanceof Error && error.message === 'request-too-large' ? 413 : 400, {
      detail: 'invalid fixture request',
    });
  }
});

server.requestTimeout = 5_000;
server.listen(port, host, () => {
  console.info(`Local auth fixture listening at http://${host}:${port}`);
});
