const path = require('node:path');
const { getDefaultConfig } = require('expo/metro-config');

const projectRoot = __dirname;
const config = getDefaultConfig(projectRoot);
const piAgentRoot = path.dirname(require.resolve('@earendil-works/pi-agent-core/package.json'));
const piAgentEntry = path.join(piAgentRoot, 'dist/agent.js');
const piAiCompatEntry = path.join(projectRoot, 'src/runtime/piAiCompat.ts');

const defaultResolveRequest = config.resolver.resolveRequest;
config.resolver.resolveRequest = (context, moduleName, platform) => {
  if (moduleName === '@earendil-works/pi-agent-core') {
    return context.resolveRequest(context, piAgentEntry, platform);
  }
  if (moduleName === '@earendil-works/pi-ai/compat') {
    return context.resolveRequest(context, piAiCompatEntry, platform);
  }
  if (defaultResolveRequest) {
    return defaultResolveRequest(context, moduleName, platform);
  }
  return context.resolveRequest(context, moduleName, platform);
};

module.exports = config;
