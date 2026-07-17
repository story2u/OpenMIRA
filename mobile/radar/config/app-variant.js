const ANDROID_MAX_VERSION_CODE = 2_100_000_000;

function resolveProductionVersion(env) {
  const version = env.RADAR_APP_VERSION?.trim();
  if (!version || !/^\d+\.\d+\.\d+$/.test(version)) {
    throw new Error(
      'Production builds require RADAR_APP_VERSION in numeric major.minor.patch form.',
    );
  }

  const buildNumber = env.RADAR_BUILD_NUMBER?.trim();
  if (!buildNumber || !/^[1-9]\d*$/.test(buildNumber)) {
    throw new Error('Production builds require RADAR_BUILD_NUMBER as a positive integer.');
  }
  const androidVersionCode = Number(buildNumber);
  if (!Number.isSafeInteger(androidVersionCode) || androidVersionCode > ANDROID_MAX_VERSION_CODE) {
    throw new Error(
      `RADAR_BUILD_NUMBER must not exceed Android versionCode limit ${ANDROID_MAX_VERSION_CODE}.`,
    );
  }

  return { version, buildNumber, androidVersionCode };
}

function resolveProductionApiBaseUrl(env) {
  const candidate = env.EXPO_PUBLIC_API_BASE_URL?.trim();
  if (!candidate) {
    throw new Error('Production builds require EXPO_PUBLIC_API_BASE_URL.');
  }

  let parsed;
  try {
    parsed = new URL(candidate);
  } catch {
    throw new Error('Production EXPO_PUBLIC_API_BASE_URL must be a valid HTTPS origin.');
  }
  if (
    parsed.protocol !== 'https:' ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash ||
    (parsed.pathname !== '' && parsed.pathname !== '/')
  ) {
    throw new Error('Production EXPO_PUBLIC_API_BASE_URL must be a valid HTTPS origin.');
  }
  return parsed.origin;
}

function resolveAppVariant(env = process.env) {
  const variant = env.RADAR_APP_VARIANT?.trim() || 'development';
  if (variant === 'development') {
    return {
      variant,
      isProduction: false,
      name: '商机雷达 P0',
      slug: 'opportunity-radar-p0',
      scheme: 'opportunity-radar-dev',
      applicationId: 'com.codeiy.im.dev',
    };
  }
  if (variant !== 'production') {
    throw new Error('RADAR_APP_VARIANT must be either development or production.');
  }

  return {
    variant,
    isProduction: true,
    name: '商机雷达',
    slug: 'opportunity-radar',
    scheme: 'opportunity-radar',
    applicationId: 'com.codeiy.im',
    ...resolveProductionVersion(env),
    apiBaseUrl: resolveProductionApiBaseUrl(env),
  };
}

module.exports = { resolveAppVariant };
