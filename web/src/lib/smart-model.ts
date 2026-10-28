export const SMART_MODEL_ID = "myapis-auto";
export const LEGACY_SMART_MODEL_ID = "veyra-auto";
export const isSmartModelId = (id: string) =>
  id === SMART_MODEL_ID || id === LEGACY_SMART_MODEL_ID;
export const canonicalModelId = (id: string) =>
  isSmartModelId(id) ? SMART_MODEL_ID : id;
