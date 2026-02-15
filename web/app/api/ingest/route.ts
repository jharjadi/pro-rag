import { NextRequest } from "next/server";
import { API_URL } from "../proxy";

export async function POST(request: NextRequest) {
  // Forward multipart form data as-is to Go API gateway
  const contentType = request.headers.get("Content-Type") || "";

  const res = await fetch(`${API_URL}/v1/ingest`, {
    method: "POST",
    headers: { "Content-Type": contentType },
    body: request.body,
    // @ts-expect-error duplex is needed for streaming request body
    duplex: "half",
  });

  const responseHeaders: Record<string, string> = {};
  const ct = res.headers.get("Content-Type");
  if (ct) responseHeaders["Content-Type"] = ct;

  return new Response(res.body, {
    status: res.status,
    headers: responseHeaders,
  });
}
