export class MobileConfigurationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'MobileConfigurationError';
  }
}

const developmentHosts = new Set(['127.0.0.1', '10.0.2.2', '::1', 'localhost']);

export function parseApiBaseUrl(value: string | undefined, allowInsecureDevelopment: boolean) {
  const candidate = value?.trim();
  if (!candidate) {
    throw new MobileConfigurationError('缺少 EXPO_PUBLIC_API_BASE_URL，无法连接登录服务。');
  }

  let parsed: URL;
  try {
    parsed = new URL(candidate);
  } catch {
    throw new MobileConfigurationError('EXPO_PUBLIC_API_BASE_URL 不是有效 URL。');
  }

  const isSecure = parsed.protocol === 'https:';
  const isAllowedDevelopmentUrl =
    allowInsecureDevelopment && parsed.protocol === 'http:' && developmentHosts.has(parsed.hostname);
  if (!isSecure && !isAllowedDevelopmentUrl) {
    throw new MobileConfigurationError('移动端 API 必须使用 HTTPS；开发模式只允许本机 HTTP。');
  }
  if (parsed.username || parsed.password || parsed.search || parsed.hash) {
    throw new MobileConfigurationError('移动端 API URL 不得包含凭据、查询参数或 fragment。');
  }
  if (parsed.pathname !== '/' && parsed.pathname !== '') {
    throw new MobileConfigurationError('EXPO_PUBLIC_API_BASE_URL 只填写服务 origin，不包含 /api/v1。');
  }
  return parsed.origin;
}
