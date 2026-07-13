'use client'

import { Loader2 } from 'lucide-react'
import { useRouter } from 'next/navigation'
import { useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { BrandLogo } from '@/components/brand-logo'
import { fetchOAuthAuthorizeUrl } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import type { OAuthProvider } from '@/lib/types'

const providerLabels: Record<OAuthProvider, string> = {
  google: '使用 Google 账号继续',
  apple: '使用 Apple 账号继续',
}

const googleIcon = (
  <svg viewBox="0 0 24 24" aria-hidden="true" className="size-5.5 shrink-0">
    <path
      fill="#4285F4"
      d="M21.6 12.23c0-.71-.06-1.4-.18-2.07H12v3.92h5.38a4.6 4.6 0 0 1-2 3.02v2.54h3.24c1.9-1.75 2.98-4.32 2.98-7.41Z"
    />
    <path
      fill="#34A853"
      d="M12 22c2.7 0 4.98-.9 6.63-2.36l-3.25-2.54c-.9.6-2.05.96-3.38.96-2.61 0-4.82-1.76-5.61-4.13H3.04v2.62A10 10 0 0 0 12 22Z"
    />
    <path
      fill="#FBBC05"
      d="M6.39 13.93A6.02 6.02 0 0 1 6.08 12c0-.67.11-1.32.31-1.93V7.45H3.04A10 10 0 0 0 2 12c0 1.61.38 3.14 1.04 4.55l3.35-2.62Z"
    />
    <path
      fill="#EA4335"
      d="M12 5.94c1.47 0 2.79.5 3.83 1.5l2.87-2.87A9.63 9.63 0 0 0 12 2a10 10 0 0 0-8.96 5.45l3.35 2.62C7.18 7.7 9.39 5.94 12 5.94Z"
    />
  </svg>
)

const appleIcon = (
  <span
    aria-hidden="true"
    className="shrink-0 font-sans text-[1.7rem] leading-none"
  >
    
  </span>
)

export default function LoginPage() {
  const router = useRouter()
  const { user, completeOAuth } = useAuth()
  const [loadingProvider, setLoadingProvider] = useState<OAuthProvider | null>(null)
  const [processingCallback, setProcessingCallback] = useState(false)
  const [error, setError] = useState('')
  const handledCallbackRef = useRef(false)

  useEffect(() => {
    if (user) {
      router.replace('/')
    }
  }, [router, user])

  useEffect(() => {
    if (handledCallbackRef.current || typeof window === 'undefined') return
    const hashParams = new URLSearchParams(window.location.hash.replace(/^#/, ''))
    const token = hashParams.get('token') ?? new URLSearchParams(window.location.search).get('token')
    if (!token) return

    handledCallbackRef.current = true
    setProcessingCallback(true)
    setError('')
    completeOAuth(token)
      .then(() => router.replace('/'))
      .catch((exc) => {
        setError(exc instanceof Error ? exc.message : '登录回调失败')
        window.history.replaceState(null, '', '/login')
      })
      .finally(() => setProcessingCallback(false))
  }, [completeOAuth, router])

  async function startOAuth(provider: OAuthProvider) {
    setError('')
    setLoadingProvider(provider)
    try {
      const authorizationUrl = await fetchOAuthAuthorizeUrl(provider)
      window.location.assign(authorizationUrl)
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : '无法发起登录')
      setLoadingProvider(null)
    }
  }

  return (
    <div className="flex min-h-svh justify-center bg-[#202020] px-5 py-10 text-white sm:items-center sm:px-8 sm:py-14">
      <section className="flex w-full max-w-xl flex-col text-center" aria-labelledby="login-heading">
        <div className="mx-auto max-w-lg">
          <BrandLogo size={72} priority className="mx-auto mb-8" />
          <h1 id="login-heading" className="text-4xl font-semibold tracking-tight sm:text-5xl">
            登录或注册
          </h1>
          <p className="mt-7 text-lg leading-8 text-zinc-300 sm:mt-8 sm:text-xl">
            登录后，AI 会帮你发现群聊中的商机，并给出安全检查与跟进建议。
          </p>
        </div>

        <div className="mt-14 grid gap-4 sm:mt-16 sm:gap-5">
          <Button
            type="button"
            variant="outline"
            className="h-16 rounded-full border-white/20 bg-transparent px-6 text-base font-semibold text-white shadow-none hover:border-white/35 hover:bg-white/5 hover:text-white sm:h-[4.5rem] sm:text-lg"
            onClick={() => startOAuth('google')}
            disabled={Boolean(loadingProvider) || processingCallback}
            aria-busy={loadingProvider === 'google'}
          >
            {loadingProvider === 'google' ? <Loader2 className="size-5 animate-spin" /> : googleIcon}
            {providerLabels.google}
          </Button>
          <Button
            type="button"
            variant="outline"
            className="h-16 rounded-full border-white/20 bg-transparent px-6 text-base font-semibold text-white shadow-none hover:border-white/35 hover:bg-white/5 hover:text-white sm:h-[4.5rem] sm:text-lg"
            onClick={() => startOAuth('apple')}
            disabled={Boolean(loadingProvider) || processingCallback}
            aria-busy={loadingProvider === 'apple'}
          >
            {loadingProvider === 'apple' ? (
              <Loader2 className="size-5 animate-spin" />
            ) : (
              appleIcon
            )}
            {providerLabels.apple}
          </Button>
        </div>

        <div className="mt-6 min-h-14" aria-live="polite">
          {processingCallback ? (
            <p className="rounded-full bg-white/5 px-4 py-3 text-sm text-zinc-300">正在完成登录…</p>
          ) : null}
          {error ? (
            <p role="alert" className="rounded-2xl border border-red-400/20 bg-red-400/10 px-4 py-3 text-sm text-red-200">
              {error}
            </p>
          ) : null}
        </div>

        <p className="mt-auto pt-12 text-xs leading-5 text-zinc-500 sm:pt-16">
          继续即表示你同意使用商机雷达处理登录所需的账户信息。
        </p>
      </section>
    </div>
  )
}
