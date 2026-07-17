import type { Static, TSchema } from 'typebox';
import { Value } from 'typebox/value';

export function typeboxDecoder<const Schema extends TSchema>(schema: Schema) {
  return (value: unknown): Static<Schema> => Value.Parse(schema, value) as Static<Schema>;
}
