import { randomUUID } from 'expo-crypto';
import { useLocalSearchParams } from 'expo-router';
import type { InteractiveToolName } from '@story2u/radar-agent/interactive';
import type { InternalOpportunityStatus } from '@story2u/radar-contracts/opportunity-actions';
import {
  forwardRef,
  memo,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  ActivityIndicator,
  AppState,
  FlatList,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';

import { useSession } from '../../auth/SessionProvider';
import { getMobileApiBaseUrl } from '../../config/mobileApiConfig';
import { useI18n } from '../../i18n/I18nProvider';
import type { MessageKey } from '../../i18n/catalog';
import { initializeRadarDatabase } from '../../storage/database';
import { colors } from '../../ui/theme';
import {
  createAgentSession,
  deleteAgentSession,
  listAgentSessions,
  readAgentEntries,
  type LocalAgentEntry,
  type LocalAgentSession,
} from '../../agent/interactive/sessionStore';
import { runInteractiveTurn } from '../../agent/interactive/runInteractiveTurn';
import type {
  InteractiveSendApprovalDecision,
  InteractiveSendApprovalRequest,
} from '../../agent/interactive/approvedSend';
import { interactiveToolPresentation } from './toolPresentation';
import type {
  InteractiveAppetiteApprovalRequest,
} from '../../agent/interactive/host';

interface StreamingBubbleHandle {
  clear(): void;
  setText(text: string): void;
}

const readOnlyToolNames = new Set<InteractiveToolName>([
  'search_opportunities',
  'get_opportunity',
  'get_messages',
]);
const toolLabelKeys: Record<InteractiveToolName, MessageKey> = {
  search_opportunities: 'agent.tool.searchOpportunities',
  get_opportunity: 'agent.tool.getOpportunity',
  get_messages: 'agent.tool.getMessages',
  draft_reply: 'agent.tool.draftReply',
  update_status: 'agent.tool.updateStatus',
  claim_opportunity: 'agent.tool.claimOpportunity',
  send_reply: 'agent.tool.sendReply',
  inspect_signal_appetite: 'agent.tool.inspectAppetite',
  start_teaching_session: 'agent.tool.startTeaching',
  capture_preference_example: 'agent.tool.captureExample',
  summarize_teaching_session: 'agent.tool.summarizeTeaching',
  propose_appetite_change: 'agent.tool.proposeAppetite',
  simulate_appetite: 'agent.tool.simulateAppetite',
  apply_appetite_change: 'agent.tool.applyAppetite',
  start_shadow_mode: 'agent.tool.startShadow',
  explain_message_decision: 'agent.tool.explainDecision',
  list_suppressed_samples: 'agent.tool.listQuietZone',
  correct_message_decision: 'agent.tool.correctDecision',
  create_temporary_focus: 'agent.tool.createFocus',
  update_attention_schedule: 'agent.tool.updateSchedule',
  undo_preference_change: 'agent.tool.undoAppetite',
  compare_preference_versions: 'agent.tool.compareAppetite',
};
const statusLabelKeys: Record<InternalOpportunityStatus, MessageKey> = {
  pending_human: 'opportunity.status.pendingHuman',
  ai_auto_reply: 'opportunity.status.aiAutoReply',
  replied: 'opportunity.status.replied',
  following: 'opportunity.status.following',
  ignored: 'opportunity.status.ignored',
  closed: 'opportunity.status.closed',
};

const StreamingBubble = memo(forwardRef<StreamingBubbleHandle>(function StreamingBubble(_, ref) {
  const { t } = useI18n();
  const [text, setText] = useState('');
  useImperativeHandle(ref, () => ({ clear: () => setText(''), setText }), []);
  return (
    <View style={[styles.messageBubble, styles.assistantBubble]}>
      {text ? (
        <Text accessibilityLiveRegion="polite" style={styles.messageText}>{text}</Text>
      ) : (
        <ActivityIndicator accessibilityLabel={t('agent.streaming')} color={colors.accent} />
      )}
    </View>
  );
}));

const ApprovalCard = memo(function ApprovalCard({
  approval,
  onChangeText,
  onDecision,
}: {
  approval: { request: InteractiveSendApprovalRequest; text: string };
  onChangeText(text: string): void;
  onDecision(approved: boolean): void;
}) {
  const { t } = useI18n();
  return (
    <View accessibilityRole="summary" style={styles.approvalCard}>
      <Text style={styles.toolCardTitle}>{t('agent.approval.title')}</Text>
      <Text style={styles.toolCardBody}>
        {t('agent.approval.target', { target: approval.request.targetLabel })}
      </Text>
      <Text style={styles.toolCardMeta}>
        {t('agent.approval.channel', { channel: approval.request.channel })}
      </Text>
      <Text style={styles.approvalLabel}>{t('agent.approval.body')}</Text>
      <TextInput
        accessibilityLabel={t('agent.approval.body')}
        maxLength={4_000}
        multiline
        onChangeText={onChangeText}
        style={styles.approvalInput}
        value={approval.text}
      />
      <Text style={styles.approvalRisk}>{t('agent.approval.risk')}</Text>
      <Text style={styles.toolCardMeta}>{t('agent.approval.validity')}</Text>
      <View style={styles.approvalActions}>
        <Pressable
          accessibilityRole="button"
          onPress={() => onDecision(false)}
          style={({ pressed }) => [styles.denyButton, pressed && styles.pressed]}
        >
          <Text style={styles.cancelButtonText}>{t('agent.approval.deny')}</Text>
        </Pressable>
        <Pressable
          accessibilityRole="button"
          disabled={!approval.text.trim()}
          onPress={() => onDecision(true)}
          style={({ pressed }) => [
            styles.approveButton,
            !approval.text.trim() && styles.disabled,
            pressed && styles.pressed,
          ]}
        >
          <Text style={styles.sendButtonText}>{t('agent.approval.approve')}</Text>
        </Pressable>
      </View>
    </View>
  );
});

const AppetiteApprovalCard = memo(function AppetiteApprovalCard({
  request,
  onDecision,
}: {
  request: InteractiveAppetiteApprovalRequest;
  onDecision(approved: boolean): void;
}) {
  const { t } = useI18n();
  return (
    <View accessibilityRole="summary" style={[styles.approvalCard, styles.appetiteApprovalCard]}>
      <Text style={styles.toolCardTitle}>{t('agent.appetiteApproval.title')}</Text>
      <Text style={styles.toolCardBody}>{t('agent.appetiteApproval.version', { version: request.preferenceVersion })}</Text>
      <Text style={styles.approvalRisk}>{t('agent.appetiteApproval.risk')}</Text>
      <Text style={styles.toolCardMeta}>{t('agent.appetiteApproval.validity')}</Text>
      <View style={styles.approvalActions}>
        <Pressable accessibilityRole="button" onPress={() => onDecision(false)} style={styles.denyButton}>
          <Text style={styles.cancelButtonText}>{t('agent.appetiteApproval.deny')}</Text>
        </Pressable>
        <Pressable accessibilityRole="button" onPress={() => onDecision(true)} style={styles.approveButton}>
          <Text style={styles.sendButtonText}>{t('agent.appetiteApproval.approve')}</Text>
        </Pressable>
      </View>
    </View>
  );
});

function entryKey(entry: LocalAgentEntry) {
  return `${entry.sessionId}:${entry.seq}`;
}

const AgentEntry = memo(function AgentEntry({
  disabled,
  entry,
  onUseDraft,
}: {
  disabled: boolean;
  entry: LocalAgentEntry;
  onUseDraft(text: string): void;
}) {
  const { t } = useI18n();
  if (entry.content.type === 'user' || entry.content.type === 'assistant') {
    const user = entry.content.type === 'user';
    return (
      <View style={[
        styles.messageBubble,
        user ? styles.userBubble : styles.assistantBubble,
      ]}>
        <Text style={styles.messageText}>{entry.content.text}</Text>
      </View>
    );
  }
  if (entry.content.type === 'tool_call') {
    const name = t(toolLabelKeys[entry.content.toolName]);
    return (
      <Text style={styles.toolStatus}>
        {t(
          entry.content.toolName === 'send_reply' || entry.content.toolName === 'apply_appetite_change'
            ? 'agent.tool.awaitingApproval'
            : readOnlyToolNames.has(entry.content.toolName)
              ? 'agent.tool.reading'
              : 'agent.tool.running',
          { name },
        )}
      </Text>
    );
  }
  if (entry.content.type === 'tool_result') {
    const presentation = interactiveToolPresentation(
      entry.content.toolName,
      entry.content.result,
    );
    if (presentation.kind === 'draft_local') {
      return (
        <View accessibilityLabel={t('agent.draft.title')} style={styles.toolCard}>
          <Text style={styles.toolCardTitle}>{t('agent.draft.title')}</Text>
          <Text selectable style={styles.toolCardBody}>{presentation.draft}</Text>
          <Text style={styles.toolCardMeta}>{t('agent.draft.localOnly')}</Text>
          <Pressable
            accessibilityRole="button"
            disabled={disabled}
            onPress={() => onUseDraft(presentation.draft)}
            style={({ pressed }) => [
              styles.toolCardButton,
              disabled && styles.disabled,
              pressed && styles.pressed,
            ]}
          >
            <Text style={styles.toolCardButtonText}>{t('agent.draft.use')}</Text>
          </Pressable>
        </View>
      );
    }
    if (presentation.kind === 'status_queued') {
      return (
        <View accessibilityRole="summary" style={styles.toolCard}>
          <Text style={styles.toolCardTitle}>{t('agent.tool.updateStatus')}</Text>
          <Text style={styles.toolCardBody}>
            {t('agent.status.queued', {
              status: t(statusLabelKeys[presentation.status]),
            })}
          </Text>
        </View>
      );
    }
    if (presentation.kind === 'claim_confirmed') {
      return (
        <View accessibilityRole="summary" style={styles.toolCard}>
          <Text style={styles.toolCardTitle}>{t('agent.tool.claimOpportunity')}</Text>
          <Text style={styles.toolCardBody}>{t('agent.claim.confirmed')}</Text>
        </View>
      );
    }
    if (presentation.kind === 'reply_sent') {
      return (
        <View accessibilityRole="summary" style={styles.toolCard}>
          <Text style={styles.toolCardTitle}>{t('agent.tool.sendReply')}</Text>
          <Text style={styles.toolCardBody}>{t('agent.reply.sent')}</Text>
        </View>
      );
    }
    return <Text style={styles.toolStatus}>{t('agent.tool.complete')}</Text>;
  }
  return (
    <Text accessibilityRole="alert" style={styles.errorText}>
      {entry.content.code === 'interactive_agent_cancelled'
        ? t('agent.error.cancelled')
        : t('agent.error.failed')}
    </Text>
  );
});

export default function AgentScreen() {
  const params = useLocalSearchParams<{ prompt?: string | string[] }>();
  const { capabilities, state } = useSession();
  const { t } = useI18n();
  const [sessions, setSessions] = useState<LocalAgentSession[]>([]);
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [entries, setEntries] = useState<LocalAgentEntry[]>([]);
  const [input, setInput] = useState('');
  const [pendingUserText, setPendingUserText] = useState('');
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pendingApproval, setPendingApproval] = useState<{
    request: InteractiveSendApprovalRequest;
    text: string;
  } | null>(null);
  const [pendingAppetiteApproval, setPendingAppetiteApproval] = useState<InteractiveAppetiteApprovalRequest | null>(null);
  const pendingApprovalController = useRef<{
    settle(decision: InteractiveSendApprovalDecision): void;
  } | null>(null);
  const pendingAppetiteApprovalController = useRef<{ settle(approved: boolean): void } | null>(null);
  const consumedPrompt = useRef<string | null>(null);
  const abortController = useRef<AbortController | null>(null);
  const stream = useRef<StreamingBubbleHandle>(null);

  const ownerId = state.status === 'authenticated' ? state.user.id : null;
  const contextualPrompt = Array.isArray(params.prompt) ? params.prompt[0] : params.prompt;
  useEffect(() => {
    if (!contextualPrompt || consumedPrompt.current === contextualPrompt || running) return;
    consumedPrompt.current = contextualPrompt;
    setInput(contextualPrompt.slice(0, 8_000));
  }, [contextualPrompt, running]);
  const loadSessions = useCallback(async (preferredSessionId?: string | null) => {
    if (!ownerId) return;
    const database = await initializeRadarDatabase();
    const nextSessions = await listAgentSessions(database, ownerId);
    const nextId = preferredSessionId && nextSessions.some((item) => item.id === preferredSessionId)
      ? preferredSessionId
      : nextSessions[0]?.id ?? null;
    const nextEntries = nextId
      ? await readAgentEntries(database, ownerId, nextId, { limit: 500 })
      : [];
    setSessions(nextSessions);
    setActiveSessionId(nextId);
    setEntries(nextEntries);
  }, [ownerId]);

  useEffect(() => {
    let disposed = false;
    setLoading(true);
    void loadSessions()
      .catch(() => {
        if (!disposed) setError(t('agent.error.local'));
      })
      .finally(() => {
        if (!disposed) setLoading(false);
      });
    return () => {
      disposed = true;
      abortController.current?.abort();
    };
  }, [loadSessions, t]);

  useEffect(() => {
    if (!capabilities.agentToolsAvailable) abortController.current?.abort();
    const subscription = AppState.addEventListener('change', (nextState) => {
      if (nextState !== 'active') abortController.current?.abort();
    });
    return () => subscription.remove();
  }, [capabilities.agentToolsAvailable]);

  const selectSession = useCallback(async (sessionId: string) => {
    if (!ownerId || running) return;
    setError(null);
    setActiveSessionId(sessionId);
    setLoading(true);
    try {
      const database = await initializeRadarDatabase();
      setEntries(await readAgentEntries(database, ownerId, sessionId, { limit: 500 }));
    } catch {
      setError(t('agent.error.local'));
    } finally {
      setLoading(false);
    }
  }, [ownerId, running, t]);

  const createSession = useCallback(async () => {
    if (!ownerId || running) return;
    setError(null);
    try {
      const database = await initializeRadarDatabase();
      const created = await createAgentSession(database, {
        ownerId,
        sessionId: randomUUID(),
        title: t('agent.session.untitled'),
      });
      await loadSessions(created.id);
    } catch {
      setError(t('agent.error.local'));
    }
  }, [loadSessions, ownerId, running, t]);

  const removeSession = useCallback(async (sessionId: string) => {
    if (!ownerId || running) return;
    setError(null);
    try {
      const database = await initializeRadarDatabase();
      await deleteAgentSession(database, ownerId, sessionId);
      await loadSessions(activeSessionId === sessionId ? null : activeSessionId);
    } catch {
      setError(t('agent.error.local'));
    }
  }, [activeSessionId, loadSessions, ownerId, running, t]);

  const submit = useCallback(async () => {
    const text = input.trim();
    if (!ownerId || !activeSessionId || !text || running) return;
    const controller = new AbortController();
    abortController.current = controller;
    setInput('');
    setPendingUserText(text);
    setError(null);
    setRunning(true);
    stream.current?.clear();
    try {
      const database = await initializeRadarDatabase();
      await runInteractiveTurn({
        baseUrl: getMobileApiBaseUrl(),
        database,
        ownerId,
        sessionId: activeSessionId,
        signal: controller.signal,
        text,
        onStreamText: (value) => stream.current?.setText(value),
        requestApproval: (request, signal) => new Promise((resolve) => {
          const settle = (decision: InteractiveSendApprovalDecision) => {
            signal?.removeEventListener('abort', denyOnAbort);
            pendingApprovalController.current = null;
            setPendingApproval(null);
            resolve(decision);
          };
          const denyOnAbort = () => settle({ approved: false, text: request.proposedText });
          if (signal?.aborted) {
            denyOnAbort();
            return;
          }
          pendingApprovalController.current?.settle({
            approved: false,
            text: request.proposedText,
          });
          pendingApprovalController.current = { settle };
          setPendingApproval({ request, text: request.proposedText });
          signal?.addEventListener('abort', denyOnAbort, { once: true });
        }),
        requestAppetiteApproval: (request, signal) => new Promise((resolve) => {
          const settle = (approved: boolean) => {
            signal?.removeEventListener('abort', denyOnAbort);
            pendingAppetiteApprovalController.current = null;
            setPendingAppetiteApproval(null);
            resolve(approved);
          };
          const denyOnAbort = () => settle(false);
          if (signal?.aborted) { denyOnAbort(); return; }
          pendingAppetiteApprovalController.current?.settle(false);
          pendingAppetiteApprovalController.current = { settle };
          setPendingAppetiteApproval(request);
          signal?.addEventListener('abort', denyOnAbort, { once: true });
        }),
      });
    } catch (caught) {
      setError(caught instanceof Error && caught.message === 'interactive_agent_cancelled'
        ? t('agent.error.cancelled')
        : t('agent.error.failed'));
    } finally {
      abortController.current = null;
      setPendingApproval(null);
      setPendingAppetiteApproval(null);
      pendingApprovalController.current = null;
      setPendingUserText('');
      setRunning(false);
      await loadSessions(activeSessionId).catch(() => setError(t('agent.error.local')));
    }
  }, [activeSessionId, input, loadSessions, ownerId, running, t]);

  const updateApprovalText = useCallback((text: string) => {
    setPendingApproval((current) => current ? { ...current, text } : null);
  }, []);

  const decideApproval = useCallback((approved: boolean) => {
    if (!pendingApproval) return;
    pendingApprovalController.current?.settle({ approved, text: pendingApproval.text });
  }, [pendingApproval]);

  const decideAppetiteApproval = useCallback((approved: boolean) => {
    pendingAppetiteApprovalController.current?.settle(approved);
  }, []);

  const useDraft = useCallback((draft: string) => {
    if (!running) setInput(draft);
  }, [running]);

  const renderEntry = useCallback(({ item }: { item: LocalAgentEntry }) => (
    <AgentEntry disabled={running} entry={item} onUseDraft={useDraft} />
  ), [running, useDraft]);

  const sessionHeader = useMemo(() => (
    <View>
      <View style={styles.headerRow}>
        <View style={styles.headerCopy}>
          <Text accessibilityRole="header" style={styles.title}>{t('agent.title')}</Text>
          <Text style={styles.subtitle}>{t('agent.subtitle')}</Text>
        </View>
        <Pressable
          accessibilityRole="button"
          disabled={running}
          onPress={() => void createSession()}
          style={({ pressed }) => [styles.smallButton, pressed && styles.pressed]}
        >
          <Text style={styles.smallButtonText}>{t('agent.session.new')}</Text>
        </Pressable>
      </View>
      <FlatList
        contentContainerStyle={styles.sessionList}
        data={sessions}
        horizontal
        keyExtractor={(item) => item.id}
        renderItem={({ item }) => (
          <View style={[
            styles.sessionChip,
            item.id === activeSessionId && styles.sessionChipActive,
          ]}>
            <Pressable
              accessibilityRole="button"
              disabled={running}
              onPress={() => void selectSession(item.id)}
            >
              <Text
                numberOfLines={1}
                style={item.id === activeSessionId
                  ? styles.sessionChipTextActive
                  : styles.sessionChipText}
              >
                {item.title}
              </Text>
            </Pressable>
            <Pressable
              accessibilityLabel={t('agent.session.delete', { title: item.title })}
              accessibilityRole="button"
              disabled={running}
              hitSlop={8}
              onPress={() => void removeSession(item.id)}
            >
              <Text style={styles.deleteMark}>×</Text>
            </Pressable>
          </View>
        )}
        showsHorizontalScrollIndicator={false}
      />
      <Text style={styles.privacy}>{t('agent.privacy')}</Text>
      {error ? <Text accessibilityRole="alert" style={styles.errorText}>{error}</Text> : null}
      {!activeSessionId && !loading ? (
        <View style={styles.empty}>
          <Text style={styles.emptyTitle}>{t('agent.empty.title')}</Text>
          <Text style={styles.subtitle}>{t('agent.empty.message')}</Text>
        </View>
      ) : null}
    </View>
  ), [
    activeSessionId,
    createSession,
    error,
    loading,
    removeSession,
    running,
    selectSession,
    sessions,
    t,
  ]);

  if (!ownerId || !capabilities.agentToolsAvailable) return null;

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      style={styles.screen}
    >
      <FlatList
        contentContainerStyle={styles.messages}
        data={entries}
        keyExtractor={entryKey}
        ListHeaderComponent={sessionHeader}
        ListFooterComponent={(
          <View>
            {pendingUserText ? (
              <View style={[styles.messageBubble, styles.userBubble]}>
                <Text style={styles.messageText}>{pendingUserText}</Text>
              </View>
            ) : null}
            {running ? <StreamingBubble ref={stream} /> : null}
            {pendingApproval ? (
              <ApprovalCard
                approval={pendingApproval}
                onChangeText={updateApprovalText}
                onDecision={decideApproval}
              />
            ) : null}
            {pendingAppetiteApproval ? (
              <AppetiteApprovalCard onDecision={decideAppetiteApproval} request={pendingAppetiteApproval} />
            ) : null}
            {loading ? <ActivityIndicator color={colors.accent} style={styles.loading} /> : null}
          </View>
        )}
        renderItem={renderEntry}
      />
      <View style={styles.composer}>
        <TextInput
          accessibilityLabel={t('agent.input.label')}
          editable={Boolean(activeSessionId) && !running}
          maxLength={8_000}
          multiline
          onChangeText={setInput}
          onSubmitEditing={() => void submit()}
          placeholder={t('agent.input.placeholder')}
          placeholderTextColor={colors.placeholder}
          style={styles.input}
          value={input}
        />
        {running ? (
          <Pressable
            accessibilityRole="button"
            onPress={() => abortController.current?.abort()}
            style={({ pressed }) => [styles.cancelButton, pressed && styles.pressed]}
          >
            <Text style={styles.cancelButtonText}>{t('common.cancel')}</Text>
          </Pressable>
        ) : (
          <Pressable
            accessibilityRole="button"
            disabled={!activeSessionId || !input.trim()}
            onPress={() => void submit()}
            style={({ pressed }) => [
              styles.sendButton,
              (!activeSessionId || !input.trim()) && styles.disabled,
              pressed && styles.pressed,
            ]}
          >
            <Text style={styles.sendButtonText}>{t('agent.send')}</Text>
          </Pressable>
        )}
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.background },
  messages: { flexGrow: 1, gap: 10, padding: 16, paddingBottom: 24 },
  headerRow: { flexDirection: 'row', alignItems: 'flex-start', gap: 12 },
  headerCopy: { flex: 1 },
  title: { color: colors.text, fontSize: 28, fontWeight: '800' },
  subtitle: { color: colors.mutedText, fontSize: 14, lineHeight: 20, marginTop: 4 },
  smallButton: { backgroundColor: colors.accentMuted, borderRadius: 10, padding: 10 },
  smallButtonText: { color: colors.accent, fontWeight: '700' },
  sessionList: { gap: 8, paddingVertical: 14 },
  sessionChip: {
    alignItems: 'center',
    backgroundColor: colors.card,
    borderColor: colors.border,
    borderRadius: 999,
    borderWidth: 1,
    flexDirection: 'row',
    gap: 8,
    maxWidth: 220,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  sessionChipActive: { backgroundColor: colors.accentMuted, borderColor: colors.accent },
  sessionChipText: { color: colors.mutedText, maxWidth: 160 },
  sessionChipTextActive: { color: colors.text, fontWeight: '700', maxWidth: 160 },
  deleteMark: { color: colors.subtleText, fontSize: 18, lineHeight: 18 },
  privacy: {
    backgroundColor: colors.noticeBackground,
    borderRadius: 10,
    color: colors.noticeText,
    fontSize: 12,
    lineHeight: 18,
    marginBottom: 12,
    padding: 10,
  },
  empty: { alignItems: 'center', paddingVertical: 48 },
  emptyTitle: { color: colors.text, fontSize: 18, fontWeight: '700' },
  messageBubble: { borderRadius: 14, maxWidth: '88%', padding: 12 },
  userBubble: { alignSelf: 'flex-end', backgroundColor: colors.button },
  assistantBubble: { alignSelf: 'flex-start', backgroundColor: colors.card },
  messageText: { color: colors.text, fontSize: 15, lineHeight: 22 },
  toolStatus: { color: colors.subtleText, fontSize: 12, marginLeft: 8 },
  toolCard: {
    alignSelf: 'flex-start',
    backgroundColor: colors.noticeBackground,
    borderColor: colors.border,
    borderRadius: 12,
    borderWidth: 1,
    gap: 8,
    maxWidth: '92%',
    padding: 12,
  },
  toolCardTitle: { color: colors.text, fontSize: 14, fontWeight: '800' },
  toolCardBody: { color: colors.text, fontSize: 14, lineHeight: 21 },
  toolCardMeta: { color: colors.noticeText, fontSize: 12, lineHeight: 17 },
  toolCardButton: {
    alignSelf: 'flex-start',
    backgroundColor: colors.accentMuted,
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  toolCardButtonText: { color: colors.accent, fontWeight: '700' },
  approvalCard: {
    alignSelf: 'stretch',
    backgroundColor: colors.noticeBackground,
    borderColor: colors.warning,
    borderRadius: 14,
    borderWidth: 1,
    gap: 9,
    marginTop: 10,
    padding: 14,
  },
  appetiteApprovalCard: { borderColor: '#6d5ca5', backgroundColor: '#211d3b' },
  approvalLabel: { color: colors.text, fontSize: 13, fontWeight: '700' },
  approvalInput: {
    backgroundColor: colors.background,
    borderColor: colors.border,
    borderRadius: 10,
    borderWidth: 1,
    color: colors.text,
    maxHeight: 180,
    minHeight: 96,
    padding: 10,
    textAlignVertical: 'top',
  },
  approvalRisk: { color: colors.noticeText, fontSize: 13, lineHeight: 19 },
  approvalActions: { flexDirection: 'row', gap: 10, justifyContent: 'flex-end' },
  denyButton: { backgroundColor: colors.errorBackground, borderRadius: 9, padding: 11 },
  approveButton: { backgroundColor: colors.accent, borderRadius: 9, padding: 11 },
  errorText: {
    backgroundColor: colors.errorBackground,
    borderRadius: 8,
    color: colors.errorText,
    lineHeight: 19,
    padding: 10,
  },
  loading: { marginVertical: 20 },
  composer: {
    alignItems: 'flex-end',
    backgroundColor: colors.card,
    borderTopColor: colors.border,
    borderTopWidth: 1,
    flexDirection: 'row',
    gap: 10,
    padding: 12,
  },
  input: {
    backgroundColor: colors.background,
    borderColor: colors.border,
    borderRadius: 12,
    borderWidth: 1,
    color: colors.text,
    flex: 1,
    maxHeight: 120,
    minHeight: 44,
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  sendButton: { backgroundColor: colors.accent, borderRadius: 10, padding: 12 },
  sendButtonText: { color: colors.background, fontWeight: '800' },
  cancelButton: { backgroundColor: colors.errorBackground, borderRadius: 10, padding: 12 },
  cancelButtonText: { color: colors.errorText, fontWeight: '700' },
  disabled: { opacity: 0.45 },
  pressed: { opacity: 0.7 },
});
