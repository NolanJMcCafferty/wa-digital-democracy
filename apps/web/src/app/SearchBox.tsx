"use client";

import Link from "next/link";
import { useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import type { SearchResult } from "@/lib/searchIndex";

const TYPE_STYLE: Record<SearchResult["type"], string> = {
  Bill: "bg-blue-100 text-blue-800",
  Hearing: "bg-amber-100 text-amber-800",
  Organization: "bg-emerald-100 text-emerald-800",
  Legislator: "bg-purple-100 text-purple-800",
  Transcript: "bg-rose-100 text-rose-800",
};

// Server-side transcript-search response shape from
// /api/v1/search/transcripts. Mirrors cmd/wa-dd-api/main.go's `body`
// type. Snake-case here matches the wire format; we camelize on use.
type TranscriptHit = {
  id: number;
  bill_id?: string;
  biennium?: string;
  bill_prefix?: string;
  bill_number?: number;
  agenda_item_label?: string;
  committee_name?: string;
  meeting_datetime?: string;
  start_ms: number;
  end_ms: number;
  text: string;
  speaker_label?: string;
  tvw_event_id?: string;
};

type TranscriptResponse = {
  query: string;
  total: number;
  limit: number;
  offset: number;
  hits: TranscriptHit[];
};

const TRANSCRIPT_DEBOUNCE_MS = 300;
const TRANSCRIPT_MIN_QUERY = 2;

export function SearchBox({ results }: { results: SearchResult[] }) {
  const [query, setQuery] = useState("");
  const deferredQuery = useDeferredValue(query);
  const trimmed = deferredQuery.trim().toLowerCase();

  // Client-side filter over the prebuilt bill/hearing/org/legislator index.
  const matches = useMemo(() => {
    if (!trimmed) return results.slice(0, 8);
    const terms = trimmed.split(/\s+/).filter(Boolean);
    return results
      .map((r) => {
        const score = terms.reduce((n, term) => n + (r.searchText.includes(term) ? 1 : 0), 0);
        return { result: r, score };
      })
      .filter((r) => r.score > 0)
      .sort((a, b) => b.score - a.score || a.result.title.localeCompare(b.result.title))
      .slice(0, 12)
      .map((r) => r.result);
  }, [results, trimmed]);

  // Debounced server-side transcript search. Cancels in-flight requests
  // when the user keeps typing.
  const [transcript, setTranscript] = useState<TranscriptResponse | null>(null);
  const [transcriptLoading, setTranscriptLoading] = useState(false);
  const [transcriptError, setTranscriptError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    // Cancel any in-flight request before starting/skipping a new one.
    abortRef.current?.abort();

    if (trimmed.length < TRANSCRIPT_MIN_QUERY) {
      setTranscript(null);
      setTranscriptLoading(false);
      setTranscriptError(null);
      return;
    }

    const controller = new AbortController();
    abortRef.current = controller;
    const timer = setTimeout(async () => {
      setTranscriptLoading(true);
      setTranscriptError(null);
      try {
        const url = `/api/v1/search/transcripts?q=${encodeURIComponent(trimmed)}&limit=20`;
        const res = await fetch(url, { signal: controller.signal });
        if (!res.ok) throw new Error(`status ${res.status}`);
        const json = (await res.json()) as TranscriptResponse;
        if (!controller.signal.aborted) {
          setTranscript(json);
        }
      } catch (err) {
        if ((err as Error).name === "AbortError") return;
        setTranscriptError((err as Error).message);
        setTranscript(null);
      } finally {
        if (!controller.signal.aborted) {
          setTranscriptLoading(false);
        }
      }
    }, TRANSCRIPT_DEBOUNCE_MS);

    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [trimmed]);

  return (
    <section
      aria-labelledby="site-search-heading"
      className="space-y-4 rounded-lg border border-stone-300 bg-white p-5"
    >
      <div className="space-y-1">
        <h2 id="site-search-heading" className="text-xl font-semibold text-stone-900">
          Search WA Digital Democracy
        </h2>
        <p className="text-sm text-stone-600">
          Search bills, hearings, organizations, legislators, and the
          words spoken in committee testimony.
        </p>
      </div>

      <label className="block">
        <span className="sr-only">Search bills, hearings, organizations, legislators, and transcripts</span>
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Try “HB 1501”, “Housing”, “WSCAI”, “Reed”, or “rent control”"
          className="w-full rounded border border-stone-300 bg-stone-50 px-3 py-2 text-stone-900 placeholder:text-stone-400 focus:border-stone-500 focus:outline-none focus:ring-2 focus:ring-stone-300"
        />
      </label>

      <div className="text-xs text-stone-500">
        {trimmed
          ? `${matches.length.toLocaleString()} entity result${matches.length === 1 ? "" : "s"}`
          : `Showing ${matches.length.toLocaleString()} of ${results.length.toLocaleString()} indexed pages`}
      </div>

      {matches.length === 0 ? (
        <p className="rounded border border-stone-200 bg-stone-50 p-3 text-sm text-stone-600">
          No matches yet. Try a bill number, legislator, organization, issue, or committee.
        </p>
      ) : (
        <ul className="divide-y divide-stone-200 rounded border border-stone-200">
          {matches.map((r) => (
            <li key={`${r.type}-${r.href}`}>
              <Link href={r.href} className="block p-3 hover:bg-stone-50">
                <div className="flex flex-col gap-1 sm:flex-row sm:items-baseline sm:justify-between">
                  <div>
                    <span className="font-medium text-stone-900">{r.title}</span>
                    <p className="text-sm text-stone-600">{r.subtitle}</p>
                  </div>
                  <span
                    className={`${TYPE_STYLE[r.type]} w-fit rounded px-2 py-0.5 text-xs font-medium uppercase tracking-wider`}
                  >
                    {r.type}
                  </span>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}

      <TranscriptResults
        trimmed={trimmed}
        response={transcript}
        loading={transcriptLoading}
        error={transcriptError}
      />
    </section>
  );
}

function TranscriptResults({
  trimmed,
  response,
  loading,
  error,
}: {
  trimmed: string;
  response: TranscriptResponse | null;
  loading: boolean;
  error: string | null;
}) {
  if (trimmed.length < TRANSCRIPT_MIN_QUERY) return null;
  return (
    <div className="space-y-2">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-sm font-medium uppercase tracking-wider text-stone-600">
          Transcript mentions
        </h3>
        <span className="text-xs text-stone-500">
          {loading
            ? "Searching…"
            : error
              ? `Error: ${error}`
              : response
                ? `${response.total.toLocaleString()} total · showing ${response.hits.length.toLocaleString()}`
                : null}
        </span>
      </div>

      {!loading && response && response.hits.length === 0 ? (
        <p className="rounded border border-stone-200 bg-stone-50 p-3 text-sm text-stone-600">
          No transcript matches. Search ranks by relevance over committee
          captions; ingestion is ongoing, so coverage is partial.
        </p>
      ) : null}

      {response && response.hits.length > 0 ? (
        <ul className="divide-y divide-stone-200 rounded border border-stone-200">
          {response.hits.map((h) => {
            const href =
              h.biennium && h.bill_prefix && h.bill_number
                ? `/bills/${h.biennium}/${h.bill_prefix}${h.bill_number}`
                : "#";
            const meeting = h.meeting_datetime
              ? new Date(h.meeting_datetime).toLocaleDateString(undefined, {
                  year: "numeric",
                  month: "short",
                  day: "numeric",
                })
              : null;
            const subtitleBits = [h.committee_name, meeting, formatTimestamp(h.start_ms)].filter(Boolean);
            return (
              <li key={h.id}>
                <Link href={href} className="block p-3 hover:bg-stone-50">
                  <div className="flex flex-col gap-1 sm:flex-row sm:items-baseline sm:justify-between">
                    <div className="min-w-0">
                      <span className="font-medium text-stone-900">
                        {h.bill_id || "Transcript"}
                        {h.agenda_item_label ? (
                          <span className="ml-2 font-normal text-stone-600">— {h.agenda_item_label}</span>
                        ) : null}
                      </span>
                      <p
                        className="text-sm leading-relaxed text-stone-700"
                        dangerouslySetInnerHTML={{ __html: highlight(h.text, trimmed) }}
                      />
                      <p className="text-xs text-stone-500">{subtitleBits.join(" · ")}</p>
                    </div>
                    <span
                      className={`${TYPE_STYLE.Transcript} w-fit rounded px-2 py-0.5 text-xs font-medium uppercase tracking-wider`}
                    >
                      Transcript
                    </span>
                  </div>
                </Link>
              </li>
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}

// HTML-escape then wrap matched terms in <mark>. We can render via
// dangerouslySetInnerHTML safely because we escape first and only
// inject literal <mark>…</mark> tags.
function highlight(snippet: string, query: string): string {
  const escaped = snippet.replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c]!));
  // Strip quote characters from the query then split into words. Keeps
  // multi-word phrase queries highlighting their constituent words.
  const cleaned = query.replace(/["']/g, " ").trim();
  const terms = cleaned.split(/\s+/).filter((t) => t.length > 1);
  if (terms.length === 0) return escaped;
  const escapedTerms = terms.map((t) => t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
  const re = new RegExp(`(${escapedTerms.join("|")})`, "gi");
  return escaped.replace(re, "<mark class=\"rounded bg-yellow-200 px-0.5\">$1</mark>");
}

// formatTimestamp renders 940368 ms → "15:43".
function formatTimestamp(ms: number): string {
  const totalSec = Math.floor(ms / 1000);
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}
