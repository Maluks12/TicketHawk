import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Emit a self-contained server build (.next/standalone) so the Docker image
  // ships only what's needed to run — much smaller, faster cold start.
  output: "standalone",
};

export default nextConfig;