import type { NextConfig } from "next";

const config: NextConfig = {
  // Default Next.js settings; the dev path reads JSON bundles directly from
  // disk via a typed import (see src/lib/loadBundle.ts). Production reads
  // from the Go API.
};

export default config;
