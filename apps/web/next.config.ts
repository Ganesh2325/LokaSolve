import path from "path";
import { loadEnvConfig } from "@next/env";
import type { NextConfig } from "next";

loadEnvConfig(path.join(__dirname, "..", ".."));

const proxyTarget = process.env.API_PROXY_TARGET ?? "http://127.0.0.1:8088";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${proxyTarget}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
