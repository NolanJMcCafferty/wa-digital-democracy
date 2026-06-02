"use client";

import Link from "next/link";
import { FormEvent, useEffect, useRef, useState } from "react";

type LookupLegislator = {
  slug: string;
  name: string;
  display_name?: string;
  role?: string;
  chamber?: string;
  district?: string;
  party?: string;
  bill_count: number;
};

type LookupResponse = {
  query_address: string;
  matched_address?: string;
  district: string;
  legislators: LookupLegislator[];
};

type AddressSuggestion = {
  text: string;
  magic_key?: string;
};

type SuggestResponse = {
  query: string;
  suggestions: AddressSuggestion[];
};

function legislatorRenderKey(legislator: LookupLegislator): string {
  return [
    legislator.slug,
    legislator.display_name ?? legislator.name,
    legislator.chamber ?? "",
    legislator.district ?? "",
  ].join("|");
}

const SUGGEST_DEBOUNCE_MS = 220;
const MIN_SUGGEST_LENGTH = 4;

export function FindLegislators({
  legislatorCount,
}: {
  legislatorCount: number;
}) {
  const [address, setAddress] = useState("");
  const [selectedMagicKey, setSelectedMagicKey] = useState<string | null>(null);
  const [suggestions, setSuggestions] = useState<AddressSuggestion[]>([]);
  const [suggestionsOpen, setSuggestionsOpen] = useState(false);
  const [suggestionsLoading, setSuggestionsLoading] = useState(false);
  const [result, setResult] = useState<LookupResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const suggestAbortRef = useRef<AbortController | null>(null);
  const canSuggest = !selectedMagicKey && address.trim().length >= MIN_SUGGEST_LENGTH;
  const visibleSuggestions = canSuggest ? suggestions : [];
  const isSuggestionsOpen = canSuggest && suggestionsOpen;
  const isSuggestionsLoading = canSuggest && suggestionsLoading;

  useEffect(() => {
    suggestAbortRef.current?.abort();
    const trimmed = address.trim();
    if (selectedMagicKey || trimmed.length < MIN_SUGGEST_LENGTH) {
      return;
    }

    const controller = new AbortController();
    suggestAbortRef.current = controller;
    const timer = setTimeout(async () => {
      setSuggestionsLoading(true);
      try {
        const res = await fetch(
          `/api/v1/addresses/suggest?query=${encodeURIComponent(trimmed)}`,
          { signal: controller.signal },
        );
        if (!res.ok) throw new Error(`status ${res.status}`);
        const json = (await res.json()) as SuggestResponse;
        if (!controller.signal.aborted) {
          setSuggestions(json.suggestions);
          setSuggestionsOpen(json.suggestions.length > 0);
        }
      } catch (err) {
        if ((err as Error).name !== "AbortError" && !controller.signal.aborted) {
          setSuggestions([]);
          setSuggestionsOpen(false);
        }
      } finally {
        if (!controller.signal.aborted) {
          setSuggestionsLoading(false);
        }
      }
    }, SUGGEST_DEBOUNCE_MS);

    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [address, selectedMagicKey]);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await lookupAddress(address, selectedMagicKey);
  }

  async function lookupAddress(value: string, magicKey?: string | null) {
    const trimmed = value.trim();
    if (trimmed.length < 5) {
      setError("Enter a Washington street address.");
      setResult(null);
      return;
    }

    setLoading(true);
    setError(null);
    setSuggestionsOpen(false);
    try {
      const params = new URLSearchParams({ address: trimmed });
      if (magicKey) params.set("magic_key", magicKey);
      const res = await fetch(`/api/v1/legislators/lookup?${params.toString()}`);
      const json = (await res.json()) as LookupResponse | { error?: string };
      if (!res.ok) {
        throw new Error("error" in json && json.error ? json.error : `status ${res.status}`);
      }
      setResult(json as LookupResponse);
    } catch (err) {
      setResult(null);
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }

  function onAddressChange(value: string) {
    setAddress(value);
    setSelectedMagicKey(null);
  }

  async function onSuggestionClick(suggestion: AddressSuggestion) {
    setAddress(suggestion.text);
    setSelectedMagicKey(suggestion.magic_key ?? null);
    setSuggestionsOpen(false);
    await lookupAddress(suggestion.text, suggestion.magic_key);
  }

  return (
    <section aria-labelledby="find-rep-heading" className="space-y-3">
      <div className="flex items-baseline justify-between gap-4">
        <h2 id="find-rep-heading" className="text-2xl font-bold text-stone-900">
          Find Your Legislators
        </h2>
        <Link
          href="/legislators"
          className="text-sm font-medium text-blue-700 underline hover:text-blue-900"
        >
          Browse all
        </Link>
      </div>

      <div className="space-y-4 rounded-lg border border-stone-300 bg-white p-5">
        <form onSubmit={onSubmit} className="space-y-3">
          <label className="block space-y-1">
            <span className="font-medium text-stone-900">Address</span>
            <span className="sr-only">Find legislators for an address</span>
            <div className="flex flex-col gap-2 sm:flex-row">
              <div className="relative min-w-0 flex-1">
                <input
                  type="search"
                  value={address}
                  onChange={(event) => onAddressChange(event.target.value)}
                  onFocus={() => {
                    if (visibleSuggestions.length > 0) setSuggestionsOpen(true);
                  }}
                  placeholder="600 4th Ave, Seattle, WA 98104"
                  autoComplete="street-address"
                  role="combobox"
                  aria-autocomplete="list"
                  aria-controls="address-suggestions"
                  aria-expanded={isSuggestionsOpen}
                  className="w-full rounded border border-stone-300 bg-stone-50 px-3 py-2 text-stone-900 placeholder:text-stone-400 focus:border-stone-500 focus:outline-none focus:ring-2 focus:ring-stone-300"
                />
                {isSuggestionsOpen ? (
                  <ul
                    id="address-suggestions"
                    className="absolute z-20 mt-1 max-h-64 w-full overflow-auto rounded border border-stone-300 bg-white shadow-lg"
                  >
                    {visibleSuggestions.map((suggestion) => (
                      <li key={`${suggestion.text}-${suggestion.magic_key ?? ""}`}>
                        <button
                          type="button"
                          onMouseDown={(event) => event.preventDefault()}
                          onClick={() => {
                            void onSuggestionClick(suggestion);
                          }}
                          className="block w-full px-3 py-2 text-left text-sm text-stone-800 hover:bg-stone-100 focus:bg-stone-100 focus:outline-none"
                        >
                          {suggestion.text}
                        </button>
                      </li>
                    ))}
                  </ul>
                ) : null}
              </div>
              <button
                type="submit"
                disabled={loading}
                className="rounded bg-stone-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-stone-700 disabled:cursor-wait disabled:bg-stone-400"
              >
                {loading ? "Searching" : "Search"}
              </button>
            </div>
          </label>
          {isSuggestionsLoading ? (
            <div className="text-xs text-stone-500">Finding address matches...</div>
          ) : null}
        </form>

        {error ? (
          <p className="rounded border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
            {error}
          </p>
        ) : null}

        {result ? (
          <div className="space-y-3">
            <div className="rounded border border-stone-200 bg-stone-50 p-3 text-sm text-stone-700">
              <div className="font-medium text-stone-900">
                Legislative District {result.district}
              </div>
              {result.matched_address ? (
                <div className="mt-1">Matched: {result.matched_address}</div>
              ) : null}
            </div>

            {result.legislators.length === 0 ? (
              <p className="rounded border border-stone-200 bg-stone-50 p-3 text-sm text-stone-600">
                District found, but no local legislator roster records are available for
                that district yet. The current index has {legislatorCount.toLocaleString()}{" "}
                legislators.
              </p>
            ) : (
              <ul className="divide-y divide-stone-200 rounded border border-stone-200">
                {result.legislators.map((legislator) => (
                  <li key={legislatorRenderKey(legislator)}>
                    <Link
                      href={`/legislators/${legislator.slug}`}
                      className="block p-3 hover:bg-stone-50"
                    >
                      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                        <div className="min-w-0">
                          <div className="font-medium text-stone-900">
                            {legislator.display_name || legislator.name}
                          </div>
                          <div className="text-sm text-stone-600">
                            {legislator.role ?? legislator.chamber ?? "Legislator"}
                            {legislator.party ? ` · ${legislator.party}` : ""}
                          </div>
                        </div>
                        <div className="text-sm text-stone-600 sm:text-right">
                          <div>District {legislator.district ?? result.district}</div>
                          <div>{legislator.bill_count.toLocaleString()} sponsored bills</div>
                        </div>
                      </div>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </div>
        ) : null}
      </div>
    </section>
  );
}
