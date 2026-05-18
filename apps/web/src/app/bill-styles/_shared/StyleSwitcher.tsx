import Link from "next/link";
import type { Bundle } from "@/lib/bundle";
import { billPathForStyle, STYLE_CONFIGS, type StyleConfig } from "./styleData";

export function StyleSwitcher({ active, bundle }: { active: StyleConfig; bundle: Bundle }) {
  return (
    <nav aria-label="Style comparison" className="no-print mb-6 rounded-2xl border border-black/10 bg-white/75 p-3 shadow-sm backdrop-blur">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Style comparison</p>
          <p className="text-sm text-slate-600">Same bill data, five different product directions.</p>
        </div>
        <Link href="/" className="text-sm font-medium text-blue-700 underline">Home</Link>
      </div>
      <div className="flex flex-wrap gap-2">
        {STYLE_CONFIGS.map((s) => {
          const current = s.key === active.key;
          return (
            <Link
              key={s.key}
              href={billPathForStyle(s, bundle)}
              className={`rounded-full px-3 py-1.5 text-sm font-medium transition ${
                current ? "bg-slate-950 text-white" : "bg-white text-slate-700 ring-1 ring-slate-200 hover:bg-slate-50"
              }`}
            >
              bill{s.demoNumber}: {s.name}
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
