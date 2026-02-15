import { NextRequest } from "next/server";
import { proxyPost } from "../proxy";

export async function POST(request: NextRequest) {
  const body = await request.text();
  return proxyPost("/v1/query", body, "application/json");
}
