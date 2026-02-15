import { NextRequest } from "next/server";
import { proxyPost } from "../proxy";

export async function POST(request: NextRequest) {
  const body = await request.text();
  // Forward tenant_id from JSON body as query param for auth middleware (dev mode).
  // Auth middleware reads tenant_id from query params, not JSON body.
  const parsed = JSON.parse(body);
  const tenantId = parsed.tenant_id || "";
  const qs = tenantId ? `?tenant_id=${encodeURIComponent(tenantId)}` : "";
  return proxyPost(`/v1/query${qs}`, body, "application/json");
}
