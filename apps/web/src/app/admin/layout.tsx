import { ClerkProvider } from "@clerk/nextjs";
import { requireAdminRole } from "@/lib/adminAuth";

export default async function AdminLayout({ children }: { children: React.ReactNode }) {
  await requireAdminRole("viewer");
  return <ClerkProvider>{children}</ClerkProvider>;
}
