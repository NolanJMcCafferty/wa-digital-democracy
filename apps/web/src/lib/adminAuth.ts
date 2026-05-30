import "server-only";

import { auth, currentUser } from "@clerk/nextjs/server";
import { headers } from "next/headers";
import { notFound, redirect } from "next/navigation";

export type AdminRole = "viewer" | "reviewer" | "admin";

const ROLE_RANK: Record<AdminRole, number> = {
  viewer: 1,
  reviewer: 2,
  admin: 3,
};

export const ADMIN_TOKEN_HEADER = "X-WADD-Admin-Auth";
export const CLERK_ADMIN_JWT_TEMPLATE = "wadd-admin";

type ClerkMetadata = Record<string, unknown> | null | undefined;

export function normalizeAdminRole(value: unknown): AdminRole | null {
  if (typeof value !== "string") return null;
  const role = value.trim().toLowerCase();
  if (role === "viewer" || role === "reviewer" || role === "admin") return role;
  return null;
}

function metadataRole(metadata: ClerkMetadata): AdminRole | null {
  if (!metadata) return null;
  return normalizeAdminRole(metadata.role ?? metadata.admin_role ?? metadata.wadd_role);
}

export function hasAdminRole(role: AdminRole | null, minimum: AdminRole): boolean {
  return role !== null && ROLE_RANK[role] >= ROLE_RANK[minimum];
}

export async function requireAdminRole(minimum: AdminRole = "viewer"): Promise<{ role: AdminRole; userId: string }> {
  const session = await auth();
  if (!session.userId) {
    const headerList = await headers();
    const pathname = headerList.get("x-pathname") || "/admin/review/speakers";
    const search = headerList.get("x-search") || "";
    redirect(`/sign-in?redirect_url=${encodeURIComponent(`${pathname}${search}`)}`);
  }

  const user = await currentUser();
  const role = metadataRole(user?.publicMetadata) ?? metadataRole(user?.privateMetadata);
  if (!role || !hasAdminRole(role, minimum)) {
    notFound();
  }
  return { role, userId: session.userId };
}

export async function clerkAdminAuthHeader(): Promise<Record<string, string>> {
  const session = await auth();
  if (!session.userId) {
    throw new Error("Clerk admin session is required for admin API requests");
  }
  const token = await session.getToken({ template: CLERK_ADMIN_JWT_TEMPLATE });
  if (!token) {
    throw new Error(`${CLERK_ADMIN_JWT_TEMPLATE} Clerk JWT template did not return a token`);
  }
  return {
    [ADMIN_TOKEN_HEADER]: `Bearer ${token}`,
  };
}
