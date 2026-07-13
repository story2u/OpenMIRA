'use client'

import { BookText, LayoutGrid, LogOut, Moon, Settings, Sun, UserRound } from 'lucide-react'
import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { useTheme } from 'next-themes'
import { useEffect, useState } from 'react'
import { ModeStatusBar } from '@/components/mode-status-bar'
import { BrandLogo } from '@/components/brand-logo'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/utils'

const navItems = [
  { href: '/', label: '商机看板', icon: LayoutGrid },
  { href: '/templates', label: '回复模板', icon: BookText },
  { href: '/settings', label: '设置中心', icon: Settings },
]

function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme()
  const [mounted, setMounted] = useState(false)

  useEffect(() => setMounted(true), [])

  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}
      aria-label="切换深浅色模式"
    >
      {mounted && resolvedTheme === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
    </Button>
  )
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const router = useRouter()
  const { user, loading, logout } = useAuth()
  const isLoginPage = pathname === '/login'
  const isHomePage = pathname === '/'
  const isPublicPage = isLoginPage || isHomePage

  useEffect(() => {
    if (!loading && !user && !isPublicPage) {
      router.replace('/login')
    }
  }, [isPublicPage, loading, router, user])

  if (isLoginPage || isHomePage) {
    return <main className="min-h-svh bg-background">{children}</main>
  }

  if (loading || !user) {
    return (
      <main className="grid min-h-svh place-items-center bg-background text-sm text-muted-foreground">
        正在校验登录状态
      </main>
    )
  }

  return (
    <div className="flex min-h-svh flex-col">
      <ModeStatusBar />
      <div className="flex flex-1">
        {/* 桌面端侧边栏 */}
        <aside className="sticky top-0 hidden h-svh w-56 shrink-0 flex-col border-r bg-sidebar md:flex">
          <div className="flex items-center gap-2 px-5 py-5">
            <BrandLogo size={32} />
            <div className="leading-tight">
              <p className="text-sm font-semibold text-sidebar-foreground">商机雷达</p>
              <p className="text-[11px] text-muted-foreground">IM 商机助手</p>
            </div>
          </div>
          <nav className="flex flex-col gap-1 px-3" aria-label="主导航">
            {navItems.map((item) => {
              const active =
                item.href === '/' ? pathname === '/' || pathname.startsWith('/opportunity') : pathname.startsWith(item.href)
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    'flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                    active
                      ? 'bg-sidebar-accent text-sidebar-accent-foreground'
                      : 'text-muted-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-foreground',
                  )}
                >
                  <item.icon className="size-4" />
                  {item.label}
                </Link>
              )
            })}
          </nav>
          <div className="mt-auto flex items-center justify-between border-t px-4 py-3">
            <div className="min-w-0">
              <div className="flex items-center gap-1.5 text-xs font-medium text-sidebar-foreground">
                <UserRound className="size-3.5" />
                <span className="truncate">{user.displayName || user.email}</span>
              </div>
              <p className="mt-0.5 truncate text-[11px] text-muted-foreground">{user.email}</p>
            </div>
            <div className="flex items-center gap-1">
              <ThemeToggle />
              <Button
                variant="ghost"
                size="icon"
                aria-label="退出登录"
                onClick={() => {
                  logout()
                  router.replace('/login')
                }}
              >
                <LogOut className="size-4" />
              </Button>
            </div>
          </div>
        </aside>

        {/* 主内容 */}
        <div className="flex min-w-0 flex-1 flex-col">
          {/* 移动端顶栏 */}
          <header className="flex items-center justify-between border-b bg-card px-4 py-3 md:hidden">
            <div className="flex items-center gap-2">
              <BrandLogo size={28} />
              <span className="text-sm font-semibold">商机雷达</span>
            </div>
            <ThemeToggle />
          </header>

          <main className="flex-1 pb-20 md:pb-0">{children}</main>

          {/* 移动端底部导航 */}
          <nav
            className="fixed inset-x-0 bottom-0 z-40 flex items-center justify-around border-t bg-card/95 py-2 backdrop-blur md:hidden"
            aria-label="底部导航"
          >
            {navItems.map((item) => {
              const active =
                item.href === '/' ? pathname === '/' || pathname.startsWith('/opportunity') : pathname.startsWith(item.href)
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    'flex flex-col items-center gap-0.5 rounded-lg px-4 py-1 text-[11px] font-medium',
                    active ? 'text-primary' : 'text-muted-foreground',
                  )}
                >
                  <item.icon className="size-5" />
                  {item.label}
                </Link>
              )
            })}
          </nav>
        </div>
      </div>
    </div>
  )
}
