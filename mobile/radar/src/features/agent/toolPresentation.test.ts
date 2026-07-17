import { describe, expect, it } from 'vitest';

import { interactiveToolPresentation } from './toolPresentation';

describe('interactiveToolPresentation', () => {
  it('renders an explicitly local, unsent draft as an editable draft card', () => {
    expect(interactiveToolPresentation('draft_reply', {
      draft: 'A concise reply',
      sent: false,
      state: 'local_only',
    })).toEqual({ draft: 'A concise reply', kind: 'draft_local' });
  });

  it('keeps queued status distinct from a confirmed server mutation', () => {
    expect(interactiveToolPresentation('update_status', {
      state: 'queued',
      status: 'following',
    })).toEqual({ kind: 'status_queued', status: 'following' });
  });

  it('renders only an explicitly confirmed claim as complete', () => {
    expect(interactiveToolPresentation('claim_opportunity', {
      claimed: true,
      state: 'confirmed',
    })).toEqual({ kind: 'claim_confirmed' });
  });

  it('renders a sent reply only after server-confirmed execution', () => {
    expect(interactiveToolPresentation('send_reply', {
      approval_id: '41234567-89ab-cdef-0123-456789abcdef',
      sent: true,
      state: 'sent',
    })).toEqual({ kind: 'reply_sent' });
    expect(interactiveToolPresentation('send_reply', {
      sent: true,
      state: 'pending',
    })).toEqual({ kind: 'complete' });
  });

  it('falls back to a generic completion instead of exposing malformed raw JSON', () => {
    expect(interactiveToolPresentation('draft_reply', {
      draft: 'secret-looking provider payload',
      sent: true,
      state: 'local_only',
    })).toEqual({ kind: 'complete' });
    expect(interactiveToolPresentation('update_status', {
      state: 'queued',
      status: 'invented',
    })).toEqual({ kind: 'complete' });
  });
});
