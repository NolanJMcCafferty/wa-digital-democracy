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

export default clerkMiddleware(async (auth, req) => {
  const requestHeaders = new Headers(req.headers);
  requestHeaders.set("x-pathname", req.nextUrl.pathname);
  requestHeaders.set("x-search", req.nextUrl.search);
  requestHeaders.set("x-public-origin", forwardedOrigin(req));

  if (isAdminRoute(req)) {
    const { userId, redirectToSignIn } = await auth();
    if (!userId) {
      const returnBackUrl = currentPublicUrl(req, `${req.nextUrl.pathname}${req.nextUrl.search}`);
      return redirectToSignIn({ returnBackUrl });
    }
  }

  return NextResponse.next({ request: { headers: requestHeaders } });
});

export const config = {
  // Only admin pages need Clerk. Public pages and health checks must remain
  // independent of Clerk env so a missing/misconfigured key cannot take down
  // the public site or Railway health checks.
  matcher: ["/admin(.*)"],
};
