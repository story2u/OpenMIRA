import type { components, paths } from './openapi';

export type ChatMessage = components['schemas']['ChatMessageRead'];
export type MessagePage = components['schemas']['MessagePageRead'];
export type MessagePageQuery = NonNullable<
  paths['/api/v1/messages/page']['get']['parameters']['query']
>;
