import { api } from "@/lib/http";
export const redeem = (code: string) =>
  api<{ quota: number }>("/user/redeem", { method: "POST", body: { code } });
