import { NextRequest } from "next/server";
import { proxyPost } from "../../../proxy";

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id } = await params;
  const search = request.nextUrl.searchParams.toString();
  return proxyPost(`/v1/documents/${id}/deactivate?${search}`, "", "application/json");
}
