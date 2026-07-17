const { resolveAppVariant } = require('./config/app-variant');

module.exports = ({ config }) => {
  const appVariant = resolveAppVariant();
  const googleServerClientId = process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID?.trim();
  const googleIosClientId = process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID?.trim();
  const googleServicesFile = process.env.RADAR_GOOGLE_SERVICES_FILE?.trim();

  if (Boolean(googleServerClientId) !== Boolean(googleIosClientId)) {
    throw new Error(
      'Google sign-in builds require both EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID and EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID.',
    );
  }

  const plugins = (config.plugins ?? []).map((plugin) => {
    if (Array.isArray(plugin) && plugin[0] === 'expo-notifications') {
      return [
        plugin[0],
        {
          ...(plugin[1] ?? {}),
          mode: appVariant.isProduction ? 'production' : 'development',
        },
      ];
    }
    if (!Array.isArray(plugin) || plugin[0] !== 'expo-build-properties') return plugin;
    const options = plugin[1] ?? {};
    return [
      plugin[0],
      {
        ...options,
        android: {
          ...(options.android ?? {}),
          usesCleartextTraffic: !appVariant.isProduction,
        },
      },
    ];
  });
  if (googleServerClientId && googleIosClientId) {
    plugins.push([
      './plugins/with-google-sign-in',
      { iosClientId: googleIosClientId, serverClientId: googleServerClientId },
    ]);
  }
  if (appVariant.isProduction) {
    plugins.push('./plugins/with-production-network-policy');
  }

  const infoPlist = { ...(config.ios?.infoPlist ?? {}) };
  if (appVariant.isProduction) {
    delete infoPlist.NSAppTransportSecurity;
  } else {
    infoPlist.NSAppTransportSecurity = { NSAllowsLocalNetworking: true };
  }

  return {
    ...config,
    name: appVariant.name,
    slug: appVariant.slug,
    scheme: appVariant.scheme,
    version: appVariant.version ?? config.version,
    ios: {
      ...config.ios,
      bundleIdentifier: appVariant.applicationId,
      buildNumber: appVariant.buildNumber ?? config.ios?.buildNumber,
      infoPlist,
    },
    android: {
      ...config.android,
      package: appVariant.applicationId,
      versionCode: appVariant.androidVersionCode ?? config.android?.versionCode,
      googleServicesFile: googleServicesFile ?? config.android?.googleServicesFile,
    },
    plugins,
  };
};
