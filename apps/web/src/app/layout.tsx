import type { Metadata } from "next";
import "./globals.css";
import { Nav } from "./Nav";

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
      <body className="min-h-screen bg-stone-50 antialiased">
        <header className="border-b border-stone-300 bg-white">
          <div className="mx-auto flex max-w-6xl flex-col gap-3 px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
            <a
              href="/"
              className="text-xl font-bold tracking-tight text-stone-900 hover:text-stone-700"
            >
              WA Digital Democracy
            </a>
            <Nav />
          </div>
        </header>
        <main className="mx-auto max-w-6xl px-6 py-10">{children}</main>
      </body>
    </html>
  );
}
