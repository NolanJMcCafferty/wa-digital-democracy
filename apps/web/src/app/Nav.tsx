"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";

// Live issue pages. The dropdown reads as a roadmap as more issue
// pages land; when Issues becomes a single link again it can collapse
// back to the simple <a> form below.
const ISSUE_LINKS = [
  { href: "/issues/climate", label: "Climate" },
  { href: "/issues/education", label: "Education" },
  { href: "/issues/health", label: "Health" },
  { href: "/issues/housing", label: "Housing" },
  { href: "/issues/public-safety", label: "Public Safety" },
  { href: "/issues/transportation", label: "Transportation" },
];

const FLAT_LINKS = [
  { href: "/bills", label: "Bills" },
  { href: "/hearings", label: "Hearings" },
  { href: "/organizations", label: "Organizations" },
  { href: "/legislators", label: "Legislators" },
  { href: "/methodology", label: "Methodology" },
];

export function Nav() {
  const [issuesOpen, setIssuesOpen] = useState(false);
  const wrapperRef = useRef<HTMLDivElement>(null);

  // Close on outside click + Escape, the standard dropdown affordances.
  useEffect(() => {
    if (!issuesOpen) return;
    const onClick = (e: MouseEvent) => {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setIssuesOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setIssuesOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [issuesOpen]);

  return (
    <nav className="flex flex-wrap items-center gap-4 text-sm font-medium text-stone-600 sm:gap-6">
      <div ref={wrapperRef} className="relative">
        <button
          type="button"
          aria-haspopup="menu"
          aria-expanded={issuesOpen}
          onClick={() => setIssuesOpen((v) => !v)}
          className="inline-flex cursor-pointer items-center gap-1 hover:text-stone-900 hover:underline"
        >
          Issues
          <span aria-hidden className="text-xs">
            {issuesOpen ? "▴" : "▾"}
          </span>
        </button>
        {issuesOpen ? (
          <div
            role="menu"
            className="absolute left-0 top-full z-10 mt-2 min-w-[10rem] rounded-md border border-stone-300 bg-white py-1 shadow-md"
          >
            {ISSUE_LINKS.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                role="menuitem"
                onClick={() => setIssuesOpen(false)}
                className="block px-4 py-2 text-stone-700 hover:bg-stone-100 hover:text-stone-900"
              >
                {link.label}
              </Link>
            ))}
          </div>
        ) : null}
      </div>

      {FLAT_LINKS.map((link) => (
        <a
          key={link.href}
          href={link.href}
          className="hover:text-stone-900 hover:underline"
        >
          {link.label}
        </a>
      ))}
    </nav>
  );
}
