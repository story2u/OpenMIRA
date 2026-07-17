import { useCallback, useEffect, useReducer, useRef, useState } from 'react';

import {
  annotateTeachingExample,
  captureTeachingExample,
  completeTeachingSession,
  startTeachingSession,
  undoTeachingExamples,
  type TeachingMessageCard,
} from '../../attention/teachingService';
import {
  applyPreferenceVersion,
  proposeAppetiteFromTeaching,
  simulatePreferenceVersion,
} from '../../attention/appetiteService';
import type {
  AppetiteSimulationSummary,
  AttentionPreference,
} from '@story2u/radar-core/attention/model';
import {
  hasSeenTeachingOnboarding,
  setTeachingOnboardingSeen,
} from '../../attention/signalAppetiteUiState';
import { useSession } from '../../auth/SessionProvider';
import { currentDeviceIdStore } from '../../device/deviceSessionStorage';
import { initializeRadarDatabase } from '../../storage/database';
import {
  initialTeachingMachineState,
  teachingReducer,
} from './teaching-machine';

export function useTeachingSession() {
  const { state: sessionState } = useSession();
  const ownerId = sessionState.status === 'authenticated' ? sessionState.user.id : null;
  const [state, dispatch] = useReducer(teachingReducer, initialTeachingMachineState);
  const [cards, setCards] = useState<TeachingMessageCard[]>([]);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [summary, setSummary] = useState<{ increase: string[]; reduce: string[] } | null>(null);
  const [proposal, setProposal] = useState<AttentionPreference | null>(null);
  const [simulation, setSimulation] = useState<AppetiteSimulationSummary | null>(null);
  const [lastExampleId, setLastExampleId] = useState<string | null>(null);
  const [applied, setApplied] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const mounted = useRef(true);

  const fail = useCallback((error: unknown) => {
    dispatch({
      type: 'FAIL',
      error: error instanceof Error ? error.message : 'teaching_failed',
    });
  }, []);

  const begin = useCallback(async (markOnboardingSeen = true) => {
    if (!ownerId) return;
    dispatch({ type: 'LOAD' });
    try {
      const [database, deviceId] = await Promise.all([
        initializeRadarDatabase(),
        currentDeviceIdStore.read(),
      ]);
      if (!deviceId) throw new Error('teaching_device_unavailable');
      if (markOnboardingSeen) await setTeachingOnboardingSeen(database, ownerId, true);
      const started = await startTeachingSession(database, { ownerId, deviceId });
      if (!started.cards.length) throw new Error('teaching_no_messages');
      if (!mounted.current) return;
      setCards(started.cards);
      setSessionId(started.sessionId);
      setSummary(null);
      setProposal(null);
      setSimulation(null);
      setLastExampleId(null);
      setApplied(false);
      setPreparing(false);
      dispatch({ type: 'READY' });
    } catch (error) {
      if (mounted.current) fail(error);
    }
  }, [fail, ownerId]);

  useEffect(() => {
    mounted.current = true;
    if (!ownerId) return () => { mounted.current = false; };
    dispatch({ type: 'LOAD' });
    void initializeRadarDatabase()
      .then(async (database) => {
        if (await hasSeenTeachingOnboarding(database, ownerId)) {
          await begin(false);
        } else if (mounted.current) {
          dispatch({ type: 'SHOW_ONBOARDING' });
        }
      })
      .catch(fail);
    return () => { mounted.current = false; };
  }, [begin, fail, ownerId]);

  const complete = useCallback(async () => {
    if (!ownerId || !sessionId || preparing) return;
    setPreparing(true);
    try {
      const [database, deviceId] = await Promise.all([
        initializeRadarDatabase(),
        currentDeviceIdStore.read(),
      ]);
      if (!deviceId) throw new Error('teaching_device_unavailable');
      const result = await completeTeachingSession(database, {
        ownerId,
        deviceId,
        sessionId,
      });
      const candidate = await proposeAppetiteFromTeaching(database, {
        ownerId,
        deviceId,
        sessionId,
      });
      const preview = await simulatePreferenceVersion(database, {
        ownerId,
        deviceId,
        version: candidate.preference.version,
      });
      if (!mounted.current) return;
      setSummary({ increase: [...result.increase], reduce: [...result.reduce] });
      setProposal(candidate.preference);
      setSimulation(preview);
      dispatch({ type: 'COMPLETE' });
    } catch (error) {
      if (mounted.current) fail(error);
    } finally {
      if (mounted.current) setPreparing(false);
    }
  }, [fail, ownerId, preparing, sessionId]);

  const capture = useCallback(async (label: 'positive' | 'negative' | 'skipped') => {
    const card = cards[state.cardIndex];
    if (!ownerId || !sessionId || !card) return;
    dispatch({ type: 'COMMIT', label });
    try {
      const [database, deviceId] = await Promise.all([
        initializeRadarDatabase(),
        currentDeviceIdStore.read(),
      ]);
      if (!deviceId) throw new Error('teaching_device_unavailable');
      const example = await captureTeachingExample(database, {
        ownerId,
        deviceId,
        sessionId,
        messageId: card.messageId,
        label,
      });
      if (!mounted.current) return;
      const finished = state.cardIndex + 1 >= cards.length;
      setLastExampleId(example.id);
      dispatch({ type: 'ADVANCE', finished });
    } catch (error) {
      if (mounted.current) fail(error);
    }
  }, [cards, fail, ownerId, sessionId, state.cardIndex]);

  const undo = useCallback(async () => {
    if (!ownerId || !sessionId || state.completedActions.length === 0) return;
    dispatch({ type: 'UNDO' });
    try {
      const [database, deviceId] = await Promise.all([
        initializeRadarDatabase(),
        currentDeviceIdStore.read(),
      ]);
      if (!deviceId) throw new Error('teaching_device_unavailable');
      await undoTeachingExamples(database, { ownerId, deviceId, sessionId, count: 1 });
      if (!mounted.current) return;
      setSummary(null);
      setLastExampleId(null);
      dispatch({ type: 'UNDO_COMPLETE' });
    } catch (error) {
      if (mounted.current) fail(error);
    }
  }, [fail, ownerId, sessionId, state.completedActions.length]);

  const annotate = useCallback(async (reasons: readonly string[], freeformReason: string | null) => {
    if (!ownerId || !sessionId || !lastExampleId) return;
    try {
      const [database, deviceId] = await Promise.all([
        initializeRadarDatabase(),
        currentDeviceIdStore.read(),
      ]);
      if (!deviceId) throw new Error('teaching_device_unavailable');
      await annotateTeachingExample(database, {
        ownerId,
        deviceId,
        sessionId,
        exampleId: lastExampleId,
        reasons,
        freeformReason,
      });
    } catch (error) {
      if (mounted.current) fail(error);
    }
  }, [fail, lastExampleId, ownerId, sessionId]);

  const apply = useCallback(async () => {
    if (!ownerId || !proposal) return;
    try {
      const [database, deviceId] = await Promise.all([
        initializeRadarDatabase(),
        currentDeviceIdStore.read(),
      ]);
      if (!deviceId) throw new Error('teaching_device_unavailable');
      await applyPreferenceVersion(database, {
        ownerId,
        deviceId,
        version: proposal.version,
        confirmed: true,
      });
      if (mounted.current) setApplied(true);
    } catch (error) {
      if (mounted.current) fail(error);
    }
  }, [fail, ownerId, proposal]);

  const setDragging = useCallback((direction: 'left' | 'right' | null) => {
    dispatch(direction
      ? { type: 'DRAG', direction }
      : { type: 'CANCEL_DRAG' });
  }, []);

  return {
    begin,
    annotate,
    apply,
    applied,
    capture,
    cards,
    complete,
    currentCard: cards[state.cardIndex] ?? null,
    lastExampleId,
    proposal,
    preparing,
    sessionId,
    state,
    summary,
    simulation,
    setDragging,
    undo,
  };
}
