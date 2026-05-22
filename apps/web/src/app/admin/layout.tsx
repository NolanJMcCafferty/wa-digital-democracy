import { requireAdminRole } from "@/lib/adminAuth";

export default async function AdminLayout({ children }: { children: React.ReactNode }) {
  await requireAdminRole("viewer");
  return children;
}
