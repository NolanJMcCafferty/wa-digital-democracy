import type { NextConfig } from "next";

const config: NextConfig = {
  // Browser-originated /api/v1/* requests are handled by
  // src/app/api/v1/[...path]/route.ts so the server can inject the internal
  // API bearer token without exposing it to client-side JavaScript.
};

export default config;
