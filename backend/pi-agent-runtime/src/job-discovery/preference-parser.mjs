export const PROTECTED_PROFILE_FIELDS = Object.freeze([
  'age', 'gender', 'race', 'ethnicity', 'religion', 'marital_status', 'disability', 'political_view',
])

export const PREFERENCE_PARSE_PROMPT = `You convert a job seeker's declared preferences into JSON.
Preserve only explicitly stated professional preferences. Never infer or include age, gender, race,
ethnicity, religion, marital status, disability, political views, or other protected attributes.
Use null or empty arrays for missing information. Do not overwrite an existing profile. The result is
only a preview and requires_confirmation must always be true.`
