const sensitiveKey = /api[-_]?key|authorization|body|content|cookie|message|password|prompt|raw[-_]?payload|secret|session|token/i;

export function redactFields(fields: Readonly<Record<string, unknown>>) {
  return Object.fromEntries(
    Object.entries(fields).map(([key, value]) => [key, sensitiveKey.test(key) ? '[REDACTED]' : value]),
  );
}

export function logEvent(event: string, fields: Readonly<Record<string, unknown>> = {}) {
  console.info(JSON.stringify({ event, ...redactFields(fields) }));
}
