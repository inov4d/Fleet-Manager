"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/api";

export interface ApiQuery<T> {
  data: T | undefined;
  error: ApiError | Error | null;
  loading: boolean;
  refetch: () => Promise<void>;
  setData: (updater: T | ((prev: T | undefined) => T | undefined)) => void;
}

/**
 * Minimal data-fetching hook. `fetcher` is re-run when `deps` change.
 * Set `enabled` to false to skip fetching.
 */
export function useApi<T>(fetcher: () => Promise<T>, deps: readonly unknown[], enabled = true): ApiQuery<T> {
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<ApiError | Error | null>(null);
  const [loading, setLoading] = useState<boolean>(enabled);
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;
  const runId = useRef(0);

  const run = useCallback(async () => {
    const id = ++runId.current;
    setLoading(true);
    try {
      const result = await fetcherRef.current();
      if (id === runId.current) {
        setData(result);
        setError(null);
      }
    } catch (e) {
      if (id === runId.current) setError(e instanceof Error ? e : new Error(String(e)));
    } finally {
      if (id === runId.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return;
    }
    void run();
    const counter = runId;
    return () => {
      counter.current++;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, run, ...deps]);

  const setDataStable = useCallback((updater: T | ((prev: T | undefined) => T | undefined)) => {
    setData((prev) => (typeof updater === "function" ? (updater as (p: T | undefined) => T | undefined)(prev) : updater));
  }, []);

  return { data, error, loading, refetch: run, setData: setDataStable };
}

/** Re-run a callback every `ms` milliseconds (for pages polling the REST API). */
export function useInterval(cb: () => void, ms: number | null) {
  const ref = useRef(cb);
  ref.current = cb;
  useEffect(() => {
    if (ms === null) return;
    const t = setInterval(() => ref.current(), ms);
    return () => clearInterval(t);
  }, [ms]);
}
