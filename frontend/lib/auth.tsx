'use client'

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { fetchMe, getAuthToken, passwordLogin, setAuthToken } from './api'
import type { AuthUser } from './types'

interface AuthContextValue {
  user: AuthUser | null
  token: string | null
  loading: boolean
  completeOAuth: (accessToken: string) => Promise<void>
  loginWithPassword: (email: string, password: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(null)
  const [user, setUser] = useState<AuthUser | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    async function restore() {
      const storedToken = getAuthToken()
      if (!storedToken) {
        setLoading(false)
        return
      }
      setToken(storedToken)
      try {
        const currentUser = await fetchMe()
        if (!cancelled) setUser(currentUser)
      } catch {
        setAuthToken(null)
        if (!cancelled) {
          setToken(null)
          setUser(null)
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    restore()
    return () => {
      cancelled = true
    }
  }, [])

  const applyAuth = useCallback((nextToken: string, nextUser: AuthUser) => {
    setAuthToken(nextToken)
    setToken(nextToken)
    setUser(nextUser)
  }, [])

  const completeOAuth = useCallback(
    async (accessToken: string) => {
      const currentUser = await fetchMe(accessToken)
      applyAuth(accessToken, currentUser)
    },
    [applyAuth],
  )

  const loginWithPassword = useCallback(
    async (email: string, password: string) => {
      const auth = await passwordLogin(email.trim().toLowerCase(), password)
      applyAuth(auth.accessToken, auth.user)
    },
    [applyAuth],
  )

  const logout = useCallback(() => {
    setAuthToken(null)
    setToken(null)
    setUser(null)
  }, [])

  const value = useMemo(
    () => ({ user, token, loading, completeOAuth, loginWithPassword, logout }),
    [user, token, loading, completeOAuth, loginWithPassword, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return ctx
}
