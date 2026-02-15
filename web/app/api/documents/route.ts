import { NextRequest } from "next/server";
import { proxyGet } from "../proxy";

export async function GET(request: NextRequest) {
  const search = request.nextUrl.searchParams.toString();
  return proxyGet(`/v1/documents?${search}`);
}
