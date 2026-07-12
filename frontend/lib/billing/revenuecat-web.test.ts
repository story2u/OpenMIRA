import { beforeEach, describe, expect, it, vi } from 'vitest'

const configure = vi.fn()

vi.mock('@revenuecat/purchases-js', () => ({
  ErrorCode: { UserCancelledError: 1 },
  PurchasesError: class PurchasesError extends Error {},
  Purchases: {
    isConfigured: () => false,
    configure,
    getSharedInstance: vi.fn(),
  },
}))

describe('RevenueCat Web initialization', () => {
  beforeEach(() => {
    vi.resetModules()
    configure.mockReset()
    vi.stubEnv('NEXT_PUBLIC_REVENUECAT_WEB_API_KEY', '')
  })

  it('fails closed without a public Web SDK key', async () => {
    const { loadWebBilling } = await import('./revenuecat-web')
    await expect(loadWebBilling('703c41d7-9abd-42ae-9778-b543b489fc51')).resolves.toEqual({ status: 'unconfigured' })
    expect(configure).not.toHaveBeenCalled()
  })

  it('never configures an anonymous user', async () => {
    vi.stubEnv('NEXT_PUBLIC_REVENUECAT_WEB_API_KEY', 'rcb_public_test')
    const { getWebPurchases } = await import('./revenuecat-web')
    await expect(getWebPurchases('')).rejects.toThrow('登录完成后才能购买套餐')
    expect(configure).not.toHaveBeenCalled()
  })
})
