// Shared proxy utility for BFF API routes.
// All browser calls go through Next.js → Go API gateway (no CORS).

const API_URL = process.env.API_URL || "http://core-api-go:8000";

export async function proxyGet(path: string): Promise<Response> {
  const res = await fetch(`${API_URL}${path}`, {
    headers: { "Content-Type": "application/json" },
    cache: "no-store",
  });
  return new Response(res.body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}

export async function proxyPost(
  path: string,
  body: BodyInit,
  contentType?: string
): Promise<Response> {
  const headers: Record<string, string> = {};
  if (contentType) {
    headers["Content-Type"] = contentType;
  }

  const res = await fetch(`${API_URL}${path}`, {
    method: "POST",
    headers,
    body,
  });

  const responseHeaders: Record<string, string> = {};
  const ct = res.headers.get("Content-Type");
  if (ct) responseHeaders["Content-Type"] = ct;

  return new Response(res.body, {
    status: res.status,
    headers: responseHeaders,
  });
}

export { API_URL };
