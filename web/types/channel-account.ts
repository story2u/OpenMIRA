export type OperationSession = {
  connected: boolean;
  label: string;
};

export type ChannelAccount = {
  accountId: string;
  accountName: string;
  agentId: string;
  deviceId: string;
  channelUserId: string;
  weworkUserId: string;
  enterpriseId: string;
  assigneeId: string;
  assigneeName: string;
  sopFlowId: string;
  sopEnabled: boolean;
  sopLabel: string;
  sopReplyWindowStart: string;
  sopReplyWindowEnd: string;
  status: string;
  aiEnabled: boolean;
  aiLabel: string;
  aiModel: string;
  knowledgeTag: string;
  createdAt: string;
  updatedAt: string;
};

export type MessageLoad = {
  assigneeId: string;
  assigneeName: string;
};

export type CreateChannelAccountInput = {
  accountId: string;
  accountName: string;
  agentId: string;
  deviceId: string;
  channelUserId: string;
  enterpriseId: string;
  sopFlowId: string;
  knowledgeTag: string;
  sopReplyWindowStart: string;
  sopReplyWindowEnd: string;
  sopEnabled: boolean;
  aiEnabled: boolean;
  aiModel: string;
  editing: boolean;
};

export type AssignAccountInput = {
  accountId: string;
  assigneeId: string;
  assigneeName: string;
};
