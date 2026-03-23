import type { Query } from "./types.js";

/**
 * createClient returns a generic protograph client that sends POST requests
 * to /protograph/v1/query on the given base URL.
 *
 * For a fully typed client, use the generated wrapper produced by
 * protograph-gen-ts (e.g. api/ts/ng_protograph.ts).
 */
export function createClient(baseURL: string) {
  return {
    async fetch(query: Query): Promise<Record<string, unknown>> {
      const url = baseURL.replace(/\/$/, "") + "/protograph/v1/query";
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(query),
      });
      if (!res.ok) {
        const text = await res.text().catch(() => String(res.status));
        throw new Error(`protograph: ${res.status} ${text}`);
      }
      return res.json() as Promise<Record<string, unknown>>;
    },
  };
}
