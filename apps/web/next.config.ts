import type { NextConfig } from "next";

const API_BASE = process.env.WADD_API_URL ?? "http://localhost:8080";

const config: NextConfig = {
  // Server Components fetch the Go API directly via WADD_API_URL.
  // This rewrite covers any future client-side fetches under
  // /api/v1/*: the browser sees same-origin, no CORS handling needed.
  async rewrites() {
    return [
      {
        source: "/api/v1/:path*",
        destination: `${API_BASE}/api/v1/:path*`,
      },
    ];
  },
};

export default config;
