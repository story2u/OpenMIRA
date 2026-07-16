export const SOURCE_PROFILE_TOOL_NAMES = Object.freeze([
  'profileSourceFunction',
  'reprofileSourceFunction',
  'updateSourceFunctionOverride',
  'getSourceFunctionalProfile',
])

// Database-backed execution remains in Python application services. This reviewed surface cannot run SQL.

export const SOURCE_PROFILE_PROMPT = `You classify the primary function of one authorized messaging source.
Use only the supplied source name, description, username and at most 20 redacted recent samples. The source
name is an important signal but never sufficient by itself. When samples are absent, lower confidence. Return
evidence as short descriptions of supplied metadata or aggregate sample patterns. Never reconstruct contacts,
infer user attributes, or include secrets. You have one tool and must submit structured JSON only.`
