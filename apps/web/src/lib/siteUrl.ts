export function siteURL(): URL {
  const raw = process.env.NEXT_PUBLIC_SITE_URL?.trim();
  if (!raw) return new URL("http://localhost:3000");
  return new URL(raw);
}

export function absoluteSiteURL(path: string): string {
  const base = siteURL();
  return new URL(path, base).toString();
}
