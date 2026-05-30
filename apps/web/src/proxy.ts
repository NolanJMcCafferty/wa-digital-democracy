import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";
import { NextResponse } from "next/server";

const isAdminRoute = createRouteMatcher(["/admin(.*)"]);

export default clerkMiddleware(async (auth, req) => {
  if (isAdminRoute(req)) {
    const { userId, redirectToSignIn } = await auth();
    if (!userId) {
      const returnBackUrl = new URL(`${req.nextUrl.pathname}${req.nextUrl.search}`, req.url).toString();
      return redirectToSignIn({ returnBackUrl });
    }
  }
  const requestHeaders = new Headers(req.headers);
  requestHeaders.set("x-pathname", req.nextUrl.pathname);
  requestHeaders.set("x-search", req.nextUrl.search);
  return NextResponse.next({ request: { headers: requestHeaders } });
});

export const config = {
  // Only admin pages need Clerk. Public pages and health checks must remain
  // independent of Clerk env so a missing/misconfigured key cannot take down
  // the public site or Railway health checks.
  matcher: ["/admin(.*)"],
};
