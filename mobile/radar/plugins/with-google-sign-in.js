const { withInfoPlist } = require('expo/config-plugins');

const googleClientIdPattern = /^[A-Za-z0-9_-]+\.apps\.googleusercontent\.com$/;

function validateClientId(name, value) {
  if (typeof value !== 'string' || !googleClientIdPattern.test(value)) {
    throw new Error(`${name} must be a Google OAuth client ID.`);
  }
}

function reversedIosClientId(clientId) {
  const suffix = '.apps.googleusercontent.com';
  return `com.googleusercontent.apps.${clientId.slice(0, -suffix.length)}`;
}

module.exports = function withGoogleSignIn(config, options) {
  validateClientId('iosClientId', options?.iosClientId);
  validateClientId('serverClientId', options?.serverClientId);

  return withInfoPlist(config, (result) => {
    const urlScheme = reversedIosClientId(options.iosClientId);
    const urlTypes = Array.isArray(result.modResults.CFBundleURLTypes)
      ? result.modResults.CFBundleURLTypes
      : [];
    const hasScheme = urlTypes.some(
      (entry) => Array.isArray(entry?.CFBundleURLSchemes)
        && entry.CFBundleURLSchemes.includes(urlScheme),
    );

    result.modResults.GIDClientID = options.iosClientId;
    result.modResults.GIDServerClientID = options.serverClientId;
    result.modResults.CFBundleURLTypes = hasScheme
      ? urlTypes
      : [...urlTypes, { CFBundleURLSchemes: [urlScheme] }];
    return result;
  });
};
