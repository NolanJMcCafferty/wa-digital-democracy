import { ClerkProvider } from "@clerk/nextjs";
import { ADMIN_AUTH_BYPASSED, requireAdminRole } from "@/lib/adminAuth";

export default async function AdminLayout({ children }: { children: React.ReactNode }) {
  await requireAdminRole("viewer");
  if (ADMIN_AUTH_BYPASSED) return <>{children}</>;
  return <ClerkProvider>{children}</ClerkProvider>;
}
