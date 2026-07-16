import { Type } from 'typebox'

const nullableString = (maxLength) => Type.Union([Type.Null(), Type.String({ maxLength })])
const nullableNumber = Type.Union([Type.Null(), Type.Number({ minimum: 0 })])
const nullableBoolean = Type.Union([Type.Null(), Type.Boolean()])
const shortStrings = (maxItems = 30) =>
  Type.Array(Type.String({ minLength: 1, maxLength: 100 }), { maxItems })

export const JobClassificationSchema = Type.Union(
  [
    'job_post', 'job_repost', 'candidate_self_promotion', 'job_seeking_request',
    'job_discussion', 'recruiter_chatter', 'referral_request', 'training_ad',
    'paid_course_ad', 'generic_ad', 'spam', 'scam', 'unrelated_chat', 'unknown',
  ].map((value) => Type.Literal(value)),
)

const SalarySchema = Type.Object(
  {
    raw: nullableString(500),
    minimum: nullableNumber,
    maximum: nullableNumber,
    currency: Type.Union([Type.Null(), Type.String({ minLength: 3, maxLength: 3 })]),
    period: Type.Union(
      ['hourly', 'daily', 'monthly', 'annual', 'project', 'unknown'].map((value) =>
        Type.Literal(value),
      ),
    ),
    negotiable: nullableBoolean,
  },
  { additionalProperties: false },
)

export const ExtractedJobSchema = Type.Object(
  {
    job_title: Type.String({ minLength: 1, maxLength: 500 }),
    normalized_job_title: nullableString(500),
    company_name: nullableString(500),
    department: nullableString(500),
    company_industry: nullableString(255),
    company_stage: nullableString(255),
    location_text: nullableString(500),
    country_code: Type.Union([Type.Null(), Type.String({ minLength: 2, maxLength: 2 })]),
    city: nullableString(255),
    timezone: nullableString(100),
    work_mode: Type.Union(
      ['remote', 'hybrid', 'on_site', 'flexible', 'unknown'].map((value) => Type.Literal(value)),
    ),
    employment_type: Type.Union(
      ['full_time', 'part_time', 'contract', 'internship', 'freelance', 'temporary', 'unknown'].map(
        (value) => Type.Literal(value),
      ),
    ),
    seniority: Type.Union(
      ['intern', 'junior', 'mid', 'senior', 'lead', 'manager', 'director', 'executive', 'unknown'].map(
        (value) => Type.Literal(value),
      ),
    ),
    salary: SalarySchema,
    equity_mentioned: nullableBoolean,
    requirements_summary: nullableString(4000),
    required_skills: shortStrings(),
    preferred_skills: shortStrings(),
    minimum_years_experience: nullableNumber,
    maximum_years_experience: nullableNumber,
    degree_required: nullableBoolean,
    degree_level: nullableString(100),
    degree_field: nullableString(255),
    english_level: nullableString(100),
    other_language_requirements: shortStrings(20),
    visa_sponsorship: nullableBoolean,
    work_authorization_text: nullableString(1000),
    relocation_support: nullableBoolean,
    age_requirement_text: nullableString(500),
    application_url: nullableString(2000),
    application_deadline: nullableString(50),
    contact_methods: Type.Array(
      Type.Object(
        {
          type: Type.String({ minLength: 1, maxLength: 50 }),
          value: Type.String({ minLength: 1, maxLength: 500 }),
        },
        { additionalProperties: false },
      ),
      { maxItems: 20 },
    ),
  },
  { additionalProperties: false },
)

export const JobAnalysisSchema = Type.Union([
  Type.Null(),
  Type.Object(
    {
      classification: JobClassificationSchema,
      classification_confidence: Type.Number({ minimum: 0, maximum: 1 }),
      noise_reasons: Type.Array(Type.String({ minLength: 1, maxLength: 500 }), { maxItems: 20 }),
      job: Type.Union([Type.Null(), ExtractedJobSchema]),
      field_evidence: Type.Record(
        Type.String({ maxLength: 100 }),
        Type.String({ minLength: 1, maxLength: 500 }),
      ),
      missing_fields: shortStrings(50),
      compliance_flags: shortStrings(20),
      extraction_confidence: Type.Number({ minimum: 0, maximum: 1 }),
    },
    { additionalProperties: false },
  ),
])
