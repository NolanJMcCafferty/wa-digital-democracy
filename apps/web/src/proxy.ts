import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";
import { NextResponse } from "next/server";

const isAdminRoute = createRouteMatcher(["/admin(.*)"]);

function forwardedOrigin(req: Request): string {
  const forwardedHost = req.headers.get("x-forwarded-host")?.split(",")[0]?.trim();
  const forwardedProto = req.headers.get("x-forwarded-proto")?.split(",")[0]?.trim();
  const host = forwardedHost || req.headers.get("host") || new URL(req.url).host;
  const proto = forwardedProto || (host.startsWith("localhost") || host.startsWith("127.0.0.1") ? "http" : "https");
  return `${proto}://${host}`;
}

function currentPublicUrl(req: Request, pathAndSearch: string): string {
  return new URL(pathAndSearch, forwardedOrigin(req)).toString();
}

function setForwardingHeaders(req: Request): Headers {
  const headers = new Headers(req.headers);
  const url = new URL(req.url);
  headers.set("x-pathname", url.pathname);
  headers.set("x-search", url.search);
  headers.set("x-public-origin", forwardedOrigin(req));
  return headers;
}

// Local development bypass: skip clerkMiddleware entirely so the admin
// pages are reachable without going through Clerk sign-in (and without
// Clerk's dev-browser handshake redirect). The admin layout and Go API
// have matching bypasses keyed off NODE_ENV.
const localDevHandler = (req: Request) =>
  NextResponse.next({ request: { headers: setForwardingHeaders(req) } });

const productionHandler = clerkMiddleware(async (auth, req) => {
  const requestHeaders = setForwardingHeaders(req);

  if (isAdminRoute(req)) {
    const { userId, redirectToSignIn } = await auth();
    if (!userId) {
      const returnBackUrl = currentPublicUrl(req, `${req.nextUrl.pathname}${req.nextUrl.search}`);
      return redirectToSignIn({ returnBackUrl });
    }
  }

  return NextResponse.next({ request: { headers: requestHeaders } });
});

export default process.env.NODE_ENV === "production" ? productionHandler : localDevHandler;

export const config = {
  // Only admin pages need Clerk. Public pages and health checks must remain
  // independent of Clerk env so a missing/misconfigured key cannot take down
  // the public site or Railway health checks.
  matcher: ["/admin(.*)"],
};
