import type { NextConfig } from "next";

const API_ORIGIN = process.env.API_ORIGIN || "http://127.0.0.1:8080";
const staticExport = process.env.STATIC_EXPORT === "1";

const nextConfig: NextConfig = staticExport
  ? { output: "export" }
  : {
      async rewrites() {
        return [
          {
            source: "/api/:path*",
            destination: `${API_ORIGIN}/api/:path*`,
          },
        ];
      },
    };

export default nextConfig;
