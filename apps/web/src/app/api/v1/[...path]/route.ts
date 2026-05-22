import { clerkAdminAuthHeader } from "@/lib/adminAuth";
import { internalAPIHeaders } from "@/lib/internalApiAuth";

export const dynamic = "force-dynamic";

const API_BASE =
  process.env.WADD_API_URL ??
  process.env.API_BASE_URL ??
  "http://localhost:8080";

const HOP_BY_HOP_HEADERS = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);


function forbidden(message: string): Response {
  return Response.json({ error: message }, { status: 403 });
}

function sameOrigin(req: Request): boolean {
  const origin = req.headers.get("origin");
  if (!origin) return true;
  try {
    const reqURL = new URL(req.url);
    return new URL(origin).host === reqURL.host;
  } catch {
    return false;
  }
}

function apiURL(path: string[], req: Request): URL {
  const base = new URL(API_BASE);
  const target = new URL(`/api/v1/${path.map(encodeURIComponent).join("/")}`, base);
  target.search = new URL(req.url).search;
  return target;
}

async function proxyHeaders(req: Request, path: string[]): Promise<Headers> {
  const headers = new Headers();
  const accept = req.headers.get("accept");
  if (accept) headers.set("accept", accept);
  const contentType = req.headers.get("content-type");
  if (contentType) headers.set("content-type", contentType);
  for (const [key, value] of Object.entries(internalAPIHeaders())) {
    headers.set(key, value);
  }
  if (path[0] === "admin") {
    for (const [key, value] of Object.entries(await clerkAdminAuthHeader())) {
      headers.set(key, value);
    }
  }
  return headers;
}

async function proxy(req: Request, path: string[], method: string): Promise<Response> {
  if (path[0] === "admin" && !["GET", "HEAD", "OPTIONS"].includes(method) && !sameOrigin(req)) {
    return forbidden("cross-origin admin mutation rejected");
  }

  const upstream = await fetch(apiURL(path, req), {
    method,
    headers: await proxyHeaders(req, path),
    body: method === "GET" || method === "HEAD" ? undefined : await req.arrayBuffer(),
    cache: "no-store",
    redirect: "manual",
  });

  const headers = new Headers();
  upstream.headers.forEach((value, key) => {
    if (!HOP_BY_HOP_HEADERS.has(key.toLowerCase())) {
      headers.set(key, value);
    }
  });
  return new Response(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers,
  });
}

type Params = { path: string[] };

export async function GET(req: Request, { params }: { params: Promise<Params> }) {
  return proxy(req, (await params).path, "GET");
}

export async function POST(req: Request, { params }: { params: Promise<Params> }) {
  return proxy(req, (await params).path, "POST");
}

export async function PUT(req: Request, { params }: { params: Promise<Params> }) {
  return proxy(req, (await params).path, "PUT");
}

export async function PATCH(req: Request, { params }: { params: Promise<Params> }) {
  return proxy(req, (await params).path, "PATCH");
}

export async function DELETE(req: Request, { params }: { params: Promise<Params> }) {
  return proxy(req, (await params).path, "DELETE");
}
