/** @type {import('next').NextConfig} */
const nextConfig = {
  typescript: {
    ignoreBuildErrors: true,
  },
  images: {
    unoptimized: true,
  },
  async rewrites() {
    const apiBaseUrl =
      process.env.INTERNAL_API_BASE_URL ??
      (process.env.NODE_ENV === 'production' ? 'http://api:8000' : 'http://127.0.0.1:8000')

    return [
      {
        source: '/api/:path*',
        destination: `${apiBaseUrl}/api/:path*`,
      },
    ]
  },
}

export default nextConfig
