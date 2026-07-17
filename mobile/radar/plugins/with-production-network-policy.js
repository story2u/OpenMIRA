const { withInfoPlist } = require('expo/config-plugins');

module.exports = function withProductionNetworkPolicy(config) {
  return withInfoPlist(config, (result) => {
    const appTransportSecurity = { ...(result.modResults.NSAppTransportSecurity ?? {}) };
    delete appTransportSecurity.NSAllowsLocalNetworking;
    if (appTransportSecurity.NSAllowsArbitraryLoads === false) {
      delete appTransportSecurity.NSAllowsArbitraryLoads;
    }

    if (Object.keys(appTransportSecurity).length === 0) {
      delete result.modResults.NSAppTransportSecurity;
    } else {
      result.modResults.NSAppTransportSecurity = appTransportSecurity;
    }
    delete result.modResults.NSBonjourServices;
    delete result.modResults.NSLocalNetworkUsageDescription;
    return result;
  });
};
