import { ResourceInventory } from "./resource-inventory";
import { useSystemAccess } from "../lib/system-access";

export function Channels({ role }: { role: number }) {
  const actualRole = useSystemAccess((state) => state.role);
  return role === actualRole ? <ResourceInventory /> : null;
}
