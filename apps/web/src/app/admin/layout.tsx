import { ClerkProvider } from "@clerk/nextjs";
import { requireAdminRole } from "@/lib/adminAuth";

export default async function AdminLayout({ children }: { children: React.ReactNode }) {
  await requireAdminRole("viewer");
  if (process.env.WADD_E2E_ADMIN_AUTH === "1") {
    return <>{children}</>;
  }
  return <ClerkProvider>{children}</ClerkProvider>;
}
