import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  devIndicators: false,
  // The accounts page died on a missing /_next/static/css file. Inlining
  // puts the stylesheet in the HTML, so a dropped CSS chunk cannot crash the app.
  experimental: {
    inlineCss: true,
  },
  turbopack: {
    root: process.cwd(),
  },
  async headers() {
    return [
      {
        source: "/_next/static/:path*",
        headers: [{ key: "Cache-Control", value: "public, max-age=31536000, immutable" }],
      },
      {
        source: "/((?!_next/static|_next/image).*)",
        headers: [
          {
            key: "Cache-Control",
            value: "private, no-cache, no-store, max-age=0, must-revalidate",
          },
        ],
      },
    ]
  },
};

export default nextConfig;
