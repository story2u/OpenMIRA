import { z } from "zod";

const timeText = z
  .string()
  .trim()
  .refine((value) => value === "" || /^([01]\d|2[0-3]):[0-5]\d$/.test(value), "请使用 HH:mm 格式");

export const channelAccountSchema = z
  .object({
    accountId: z.string().trim(),
    accountName: z.string().trim().min(1, "请输入账号名称"),
    agentId: z.string().trim(),
    deviceId: z.string().trim(),
    channelUserId: z.string().trim(),
    enterpriseId: z.string().trim(),
    sopFlowId: z.string().trim(),
    knowledgeTag: z.string().trim(),
    sopReplyWindowStart: timeText,
    sopReplyWindowEnd: timeText,
    sopEnabled: z.boolean(),
    aiEnabled: z.boolean(),
    aiModel: z.string().trim(),
    editing: z.boolean(),
  })
  .refine(
    (value) => {
      if (!value.sopReplyWindowStart || !value.sopReplyWindowEnd) return true;
      return value.sopReplyWindowStart <= value.sopReplyWindowEnd;
    },
    {
      path: ["sopReplyWindowEnd"],
      message: "SOP 结束时间不能早于开始时间",
    },
  );

export const accountAssignmentSchema = z.object({
  accountId: z.string().trim().min(1, "请选择通道账号"),
  assigneeId: z.string().trim().min(1, "请选择或输入消息端账号"),
  assigneeName: z.string().trim(),
});

export const csvImportSchema = z.object({
  file: z
    .custom<File>((value) => value instanceof File, "请选择 CSV 文件")
    .refine((file) => file.name.toLowerCase().endsWith(".csv"), "仅支持 CSV 文件"),
});

export type ChannelAccountFormValues = z.infer<typeof channelAccountSchema>;
export type AccountAssignmentFormValues = z.infer<typeof accountAssignmentSchema>;
export type CsvImportFormValues = z.infer<typeof csvImportSchema>;

export function defaultChannelAccountValues(): ChannelAccountFormValues {
  return {
    accountId: "",
    accountName: "",
    agentId: "",
    deviceId: "",
    channelUserId: "",
    enterpriseId: "",
    sopFlowId: "",
    knowledgeTag: "",
    sopReplyWindowStart: "",
    sopReplyWindowEnd: "",
    sopEnabled: false,
    aiEnabled: false,
    aiModel: "",
    editing: false,
  };
}

export function defaultAccountAssignmentValues(): AccountAssignmentFormValues {
  return {
    accountId: "",
    assigneeId: "",
    assigneeName: "",
  };
}
