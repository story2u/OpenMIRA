import { afterEach, describe, expect, it, vi } from 'vitest'
import { createJobSearchProfile, fetchJobs } from './api'
import type { JobSearchProfileInput } from './types'

afterEach(() => vi.unstubAllGlobals())

function mockJsonResponse(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const profile: JobSearchProfileInput = {
  name: '远程后端', isDefault: true, enabled: true,
  targetRoles: ['Python Backend Engineer'], excludedRoles: [], targetIndustries: [],
  preferredSeniority: ['mid'], candidateSkills: ['Python'], yearsExperience: 3,
  educationLevel: null, englishLevel: null, otherLanguages: [], preferredCountries: [],
  preferredCities: [], preferredTimezones: ['Europe/Berlin'], workModes: ['remote'],
  employmentTypes: ['full_time'], minimumSalary: 80000, salaryCurrency: 'USD',
  salaryPeriod: 'annual', visaSponsorshipRequired: true, relocationAcceptable: null,
  requiredKeywords: [], preferredKeywords: [], excludedKeywords: [],
  requireSalaryDisclosed: false, minimumMatchScore: 60, notificationEnabled: false,
}

describe('job discovery API', () => {
  it('maps list filters to server-owned query parameters', async () => {
    const fetchMock = mockJsonResponse({ items: [], total: 0, limit: 20, offset: 0, filterSummary: {}, profile: null })

    await fetchJobs({ profileId: 'profile-1', workMode: 'remote', minimumMatchScore: 70 })

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      '/api/v1/jobs?profile_id=profile-1&work_mode=remote&minimum_match_score=70',
    )
  })

  it('saves declared professional preferences without protected attributes', async () => {
    const fetchMock = mockJsonResponse({ id: 'profile-1', ...profile })

    await createJobSearchProfile(profile)

    const body = JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))
    expect(body.targetRoles).toEqual(['Python Backend Engineer'])
    expect(body.age).toBeUndefined()
    expect(body.gender).toBeUndefined()
    expect(body.race).toBeUndefined()
  })
})
