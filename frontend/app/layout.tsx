import { Analytics } from '@vercel/analytics/next'
import type { Metadata, Viewport } from 'next'
import { AppShell } from '@/components/app-shell'
import { ThemeProvider } from '@/components/theme-provider'
import { AppStoreProvider } from '@/lib/app-store'
import { AuthProvider } from '@/lib/auth'
import './globals.css'

export const metadata: Metadata = {
  title: '商机雷达 - 企业级 IM 商机助手',
  description: '连接 Telegram 与企业微信的智能商机管理工具，白天人工审核，夜间 AI 自动回复',
  generator: 'v0.app',
  metadataBase: new URL(process.env.NEXT_PUBLIC_FRONTEND_BASE_URL || 'https://im.story2u.xyz'),
  openGraph: {
    title: '商机雷达 · Opportunity Radar',
    description: '多 IM 渠道商机识别与 AI 辅助跟进工具',
  },
  icons: {
    icon: [
      {
        url: '/icon-light-32x32.png',
        media: '(prefers-color-scheme: light)',
      },
      {
        url: '/icon-dark-32x32.png',
        media: '(prefers-color-scheme: dark)',
      },
      {
        url: '/icon.svg',
        type: 'image/svg+xml',
      },
    ],
    apple: '/apple-icon.png',
  },
}

export const viewport: Viewport = {
  colorScheme: 'light dark',
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: '#f8f8fa' },
    { media: '(prefers-color-scheme: dark)', color: '#17171f' },
  ],
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="zh-CN" className="bg-background" suppressHydrationWarning>
      <body className="antialiased font-sans">
        <ThemeProvider attribute="class" defaultTheme="light" enableSystem disableTransitionOnChange>
          <AuthProvider>
            <AppStoreProvider>
              <AppShell>{children}</AppShell>
            </AppStoreProvider>
          </AuthProvider>
        </ThemeProvider>
        {process.env.VERCEL === '1' && <Analytics />}
      </body>
    </html>
  )
}
