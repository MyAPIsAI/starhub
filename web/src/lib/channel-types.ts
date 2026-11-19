interface ChannelTypeOption {
  value: number;
  label: [string, string, string];
  defaultBaseURL?: string;
  discovery?: boolean;
  chatOnly?: boolean;
}

export const channelTypeOptions: ChannelTypeOption[] = [
  { value: 1, label: ["OpenAI", "OpenAI", "OpenAI"], discovery: true },
  {
    value: 32,
    label: [
      "火山方舟（豆包）",
      "Volcengine Ark (Doubao)",
      "Volcengine Ark (Doubao)",
    ],
    discovery: true,
  },
  {
    value: 26,
    label: ["智谱（GLM）", "Zhipu (GLM)", "Zhipu (GLM)"],
    defaultBaseURL: "https://open.bigmodel.cn/api/paas/v4",
    chatOnly: true,
  },
  {
    value: 24,
    label: ["Kimi（Moonshot）", "Kimi (Moonshot)", "Kimi (Moonshot)"],
    defaultBaseURL: "https://api.moonshot.cn/v1",
    discovery: true,
    chatOnly: true,
  },
  {
    value: 45,
    label: ["DeepSeek", "DeepSeek", "DeepSeek"],
    defaultBaseURL: "https://api.deepseek.com",
    discovery: true,
    chatOnly: true,
  },
  { value: 14, label: ["Claude", "Claude", "Claude"] },
  { value: 15, label: ["AWS Bedrock", "AWS Bedrock", "AWS Bedrock"] },
  { value: 25, label: ["Gemini", "Gemini", "Gemini"] },
  {
    value: 61,
    label: ["视频任务插件", "Video task plugin", "إضافة مهام الفيديو"],
    discovery: true,
  },
];

export const channelTypeOption = (value: number) =>
  channelTypeOptions.find((option) => option.value === value);
