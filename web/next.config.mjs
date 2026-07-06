/** @type {import('next').NextConfig} */
const internalAPIBaseURL = String(process.env.IM_API_INTERNAL_BASE_URL || "http://127.0.0.1:9000").replace(/\/+$/, "");

const nextConfig = {
  reactStrictMode: true,
  async rewrites() {
    return [
      {
        source: "/api/v1/:path*",
        destination: `${internalAPIBaseURL}/api/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;
