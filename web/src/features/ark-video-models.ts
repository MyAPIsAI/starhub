export function isArkVideoModel(id: string): boolean {
  return /^(doubao-seedance-|dreamina-seedance-|seedance-)/.test(id);
}

// Reference IDs remain unverified until checked against the configured account.
export function arkVideoReferenceModels(baseURL = ""): string[] {
  let bytePlus = false;
  try {
    bytePlus = new URL(baseURL).hostname === "ark.ap-southeast.bytepluses.com";
  } catch {
    // An empty address uses the domestic Ark default.
  }
  const prefix = bytePlus ? "dreamina" : "doubao";
  return [
    `${prefix}-seedance-2-0-260128`,
    `${prefix}-seedance-2-0-fast-260128`,
    `${prefix}-seedance-2-0-mini-260615`,
    `${prefix}-seedance-2-5-260628`,
    bytePlus ? "seedance-1-0-pro-250528" : "doubao-seedance-1-0-pro-250528",
  ];
}
