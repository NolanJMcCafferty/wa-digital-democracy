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

function apiURL(path: string[], req: Request): URL {
  const base = new URL(API_BASE);
  const target = new URL(`/api/v1/${path.map(encodeURIComponent).join("/")}`, base);
  target.search = new URL(req.url).search;
  return target;
}

function proxyHeaders(req: Request): Headers {
  const headers = new Headers();
  const accept = req.headers.get("accept");
  if (accept) headers.set("accept", accept);
  const contentType = req.headers.get("content-type");
  if (contentType) headers.set("content-type", contentType);
  for (const [key, value] of Object.entries(internalAPIHeaders())) {
    headers.set(key, value);
  }
  return headers;
}

async function proxy(req: Request, path: string[], method: string): Promise<Response> {
  const upstream = await fetch(apiURL(path, req), {
    method,
    headers: proxyHeaders(req),
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
