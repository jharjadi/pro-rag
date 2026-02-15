import { NextRequest } from "next/server";
import { proxyGet } from "../../../proxy";

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id } = await params;
  const search = request.nextUrl.searchParams.toString();
  return proxyGet(`/v1/documents/${id}/chunks?${search}`);
}
