import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "WA Digital Democracy",
  description:
    "A source-linked public graph of Washington State government — bills, hearings, testimony, video, money, and lobbying.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="min-h-screen antialiased">
        <header className="border-b border-stone-300 bg-stone-50/80 backdrop-blur-sm">
          <div className="mx-auto flex max-w-5xl items-baseline justify-between px-6 py-4">
            <a href="/" className="text-lg font-semibold tracking-tight text-stone-900">
              WA Digital Democracy
            </a>
            <nav className="flex items-center gap-4 text-xs uppercase tracking-wider text-stone-500">
              <a href="/issues/housing" className="hover:text-stone-900">
                Housing
              </a>
              <a href="/hearings" className="hover:text-stone-900">
                Hearings
              </a>
              <a href="/organizations" className="hover:text-stone-900">
                Orgs
              </a>
              <span>First-page MVP</span>
            </nav>
          </div>
        </header>
        <main className="mx-auto max-w-5xl px-6 py-10">{children}</main>
        <footer className="mx-auto mt-16 max-w-5xl border-t border-stone-300 px-6 py-6 text-xs text-stone-500">
          Source-linked. Every fact carries a fetched_at timestamp and a link
          to the official record. Sources panel below each page lists all
          underlying API calls.
        </footer>
      </body>
    </html>
  );
}
