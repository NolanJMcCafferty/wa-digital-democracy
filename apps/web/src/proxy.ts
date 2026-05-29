import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";
import { NextResponse } from "next/server";
import { absoluteSiteURL } from "@/lib/siteUrl";

const isAdminRoute = createRouteMatcher(["/admin(.*)"]);

export default clerkMiddleware(async (auth, req) => {
  if (isAdminRoute(req)) {
    const { userId, redirectToSignIn } = await auth();
    if (!userId) {
      const returnBackUrl = absoluteSiteURL(`${req.nextUrl.pathname}${req.nextUrl.search}`);
      return redirectToSignIn({ returnBackUrl });
    }
  }
  return NextResponse.next();
});

export const config = {
  // Only admin pages need Clerk. Public pages and health checks must remain
  // independent of Clerk env so a missing/misconfigured key cannot take down
  // the public site or Railway health checks.
  matcher: ["/admin(.*)"],
};
