import type {
  AuthToken,
  AuthUser,
  NativeLoginRequest,
  PasswordLoginRequest,
} from '@story2u/radar-contracts/auth';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { typeboxDecoder } from './typebox-decoder';

export type OAuthProvider = 'apple' | 'google';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';

export const AuthUserSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    email: Type.String(),
    displayName: Type.String(),
    avatarUrl: Type.String(),
    isAdmin: Type.Boolean(),
  },
  { additionalProperties: false },
);

export const AuthTokenSchema = Type.Object(
  {
    accessToken: Type.String({ minLength: 16, maxLength: 16_384 }),
    tokenType: Type.Literal('bearer'),
    user: AuthUserSchema,
  },
  { additionalProperties: false },
);

const OAuthAuthorizeSchema = Type.Object(
  { authorizationUrl: Type.String({ minLength: 1 }) },
  { additionalProperties: false },
);

const decodeAuthUser: ResponseDecoder<AuthUser> = typeboxDecoder(AuthUserSchema);
const decodeAuthToken: ResponseDecoder<AuthToken> = typeboxDecoder(AuthTokenSchema);
const decodeOAuthAuthorize = typeboxDecoder(OAuthAuthorizeSchema);

function explicitAuthorization(accessToken?: string) {
  if (accessToken === undefined) return undefined;
  if (accessToken.length < 16 || accessToken.length > 16_384 || /[\r\n]/.test(accessToken)) {
    throw new Error('Invalid access token');
  }
  return { Authorization: `Bearer ${accessToken}` };
}

function explicitIdentityToken(value: string) {
  if (
    value.length < 16
    || value.length > 8192
    || value.trim() !== value
    || /[\r\n]/.test(value)
  ) {
    throw new Error('Invalid native identity token');
  }
  return value;
}

export function createAuthApi(client: RadarApiClient) {
  return {
    getCurrentUser(accessToken?: string): Promise<AuthUser> {
      return client.request('/api/v1/auth/me', {
        decode: decodeAuthUser,
        headers: explicitAuthorization(accessToken),
      });
    },

    loginWithPassword(payload: PasswordLoginRequest): Promise<AuthToken> {
      return client.request('/api/v1/auth/password/login', {
        method: 'POST',
        body: JSON.stringify({
          email: payload.email.trim().toLowerCase(),
          password: payload.password,
        } satisfies PasswordLoginRequest),
        decode: decodeAuthToken,
      });
    },

    loginWithNativeToken(provider: OAuthProvider, payload: NativeLoginRequest): Promise<AuthToken> {
      return client.request(`/api/v1/auth/oauth/${provider}/native`, {
        method: 'POST',
        body: JSON.stringify({
          idToken: explicitIdentityToken(payload.idToken),
        } satisfies NativeLoginRequest),
        decode: decodeAuthToken,
      });
    },

    async getOAuthAuthorizeUrl(provider: OAuthProvider): Promise<string> {
      const response = await client.request(`/api/v1/auth/oauth/${provider}/authorize`, {
        decode: decodeOAuthAuthorize,
      });
      return response.authorizationUrl;
    },
  };
}
