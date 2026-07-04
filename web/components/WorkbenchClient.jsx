"use client";

/*
 * CS workbench read-only client for the Next.js migration.
 * It passes filters to Go/Python-compatible APIs and renders returned facts
 * without reimplementing backend-owned business filtering in the browser.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ensureSessionTokenFresh, getSessionToken, requestSessionJSON } from "../lib/sessionToken.js";
import { consumeCSURLSession, getStoredCSAssigneeID, loginCSWithPassword, logoutSession, sessionLoginErrorMessage } from "../lib/sessionLogin.js";
import { useRealtimeChannel } from "../lib/useRealtimeChannel.js";
import {
  buildWorkbenchArchiveMediaPrepareAction,
  buildWorkbenchMediaPreview,
  buildWorkbenchVoiceTranscriptionRetryAction,
  formatWorkbenchVoiceTranscriptionRetryError,
  normalizeWorkbenchVoiceTranscriptionRetryResult,
  resolveWorkbenchMessagePresentation,
  resolveWorkbenchVoiceMediaKind,
  resolveWorkbenchVoiceTranscriptionDisplay,
} from "../lib/workbenchMessages.js";
import {
  buildConversationAIModeMutation,
  isAIReplyErrorDismissed,
  rememberDismissedAIReplyError,
  resolveConversationAIToggleState,
  resolveWorkbenchAIReplyErrorNotice,
  resolveWorkbenchConversationBadges,
} from "../lib/workbenchConversationState.js";
import {
  VOICE_RECORDING_MAX_MS,
  createVoiceRecordingFile,
  formatVoiceRecordingSeconds,
  selectVoiceRecorderMimeType,
  shouldUploadVoiceRecording,
  voiceRecordingErrorMessage,
} from "../lib/workbenchVoiceRecorder.js";
import {
  buildConversationCallPayload,
  buildConversationHangupPayload,
  buildConversationReadMutation,
  buildConversationMediaSendPayload,
  buildConversationResendPayload,
  buildConversationRevokePayload,
  buildConversationReplyPayload,
  buildSidebarMixedMessagesMutation,
  buildAISuggestionEditDraft,
  canRetryLocalMediaMessage,
  canEditAISuggestionMessage,
  canResendConversationMessage,
  canRevokeConversationMessage,
  createLocalMediaRetryMessage,
  createLocalMediaOutgoingMessage,
  createLocalOutgoingMessage,
  createResendOutgoingMessage,
  isAISuggestionConflictError,
  mergeLocalOutgoingMessages,
  nextManualTextClientBatch,
  reconcileLocalOutgoingMessage,
  resolveRevokeOccurrenceFromBottom,
} from "../lib/workbenchSend.js";
import {
  buildWorkbenchConversationLookupRequest,
  resolveWorkbenchAISuggestion,
  resolveWorkbenchConversationLookupResult,
  resolveWorkbenchRealtimeIntent,
  workbenchConversationRealtimeTopics,
  workbenchTaskRealtimeTopics,
} from "../lib/workbenchRealtime.js";

const conversationLimit = 30;
const messageLimit = 30;
const mixedMessageTypeOptions = [
  { value: "text", label: "文字" },
  { value: "image", label: "图片" },
  { value: "file", label: "文件" },
  { value: "video", label: "视频" },
];

function createMixedMessageDraft(id = "mixed-1", type = "text") {
  return { id, type, content: "" };
}

function conversationAIModeErrorMessage(error) {
  const messages = {
    conversation_required: "请选择会话",
    pending_conversation: "临时会话不能切换 AI 托管",
    enabled_required: "缺少 AI 托管目标状态",
  };
  return messages[error] || "AI 托管切换失败";
}

export function WorkbenchClient() {
  const [token, setToken] = useState("");
  const [loginAssigneeId, setLoginAssigneeId] = useState("");
  const [loginPassword, setLoginPassword] = useState("");
  const [authLoading, setAuthLoading] = useState(false);
  const [authError, setAuthError] = useState("");
  const [selectedAccountId, setSelectedAccountId] = useState("all");
  const [modeFilter, setModeFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("pending");
  const [searchKeyword, setSearchKeyword] = useState("");
  const [conversations, setConversations] = useState([]);
  const [searchResults, setSearchResults] = useState([]);
  const [conversationPage, setConversationPage] = useState(null);
  const [selectedConversation, setSelectedConversation] = useState(null);
  const [messages, setMessages] = useState([]);
  const [messagesMeta, setMessagesMeta] = useState(null);
  const [localOutgoingMessages, setLocalOutgoingMessages] = useState([]);
  const [sendDraft, setSendDraft] = useState("");
  const [aiSuggestionEdit, setAISuggestionEdit] = useState(null);
  const [sending, setSending] = useState(false);
  const [voiceRecording, setVoiceRecording] = useState(false);
  const [voiceRecordingSeconds, setVoiceRecordingSeconds] = useState(0);
  const [voiceRecordingError, setVoiceRecordingError] = useState("");
  const [mixedMessages, setMixedMessages] = useState(() => [createMixedMessageDraft()]);
  const [mixedSending, setMixedSending] = useState(false);
  const [mixedNotice, setMixedNotice] = useState("");
  const [aiToggleBusyConversation, setAIToggleBusyConversation] = useState("");
  const [aiToggleError, setAIToggleError] = useState("");
  const [dismissedAIReplyErrorKey, setDismissedAIReplyErrorKey] = useState("");
  const [callBusy, setCallBusy] = useState("");
  const [callState, setCallState] = useState(null);
  const [previewMedia, setPreviewMedia] = useState(null);
  const [revokeNowMs, setRevokeNowMs] = useState(() => Date.now());
  const [loading, setLoading] = useState({ bootstrap: false, search: false, messages: false });
  const [error, setError] = useState("");
  const [conversationRefreshNonce, setConversationRefreshNonce] = useState(0);
  const [messageRefreshNonce, setMessageRefreshNonce] = useState(0);
  const searchAbortRef = useRef(null);
  const mediaInputRef = useRef(null);
  const manualTextClientBatchRef = useRef(null);
  const voiceRecorderRef = useRef(null);
  const voiceStreamRef = useRef(null);
  const voiceChunksRef = useRef([]);
  const voiceStartedAtRef = useRef(0);
  const voiceTimerRef = useRef(0);
  const voiceShouldUploadRef = useRef(false);
  const conversationByIdRef = useRef(new Map());
  const pendingAISuggestionsRef = useRef(new Set());
  const pendingConversationLookupsRef = useRef(new Map());
  const selectedConversationIdRef = useRef("");
  const markReadKeysRef = useRef(new Set());
  const selectedConversationID = selectedConversation?.conversation_id || "";

  useEffect(() => {
    const injectedSession = consumeCSURLSession();
    const savedToken = injectedSession.token || getSessionToken("cs");
    setToken(savedToken);
    if (typeof window !== "undefined") {
      const params = new URLSearchParams(window.location.search);
      const savedAssigneeId = getStoredCSAssigneeID();
      setLoginAssigneeId(injectedSession.assignee_id || savedAssigneeId || params.get("cs_id") || "");
    }
  }, []);

  useEffect(() => {
    selectedConversationIdRef.current = selectedConversationID;
  }, [selectedConversationID]);

  useEffect(() => {
    setAIToggleError("");
  }, [selectedConversationID]);

  useEffect(() => {
    const byId = new Map();
    const addConversation = (conversation) => {
      if (!conversation || typeof conversation !== "object") return;
      [conversation.conversation_id, conversation.conversation_key, conversation.resolved_conversation_id].forEach((id) => {
        const key = String(id || "").trim();
        if (key) byId.set(key, conversation);
      });
    };
    conversations.forEach(addConversation);
    searchResults.forEach(addConversation);
    addConversation(selectedConversation);
    conversationByIdRef.current = byId;
  }, [conversations, searchResults, selectedConversation]);

  useEffect(() => {
    setCallState(null);
    setAISuggestionEdit(null);
    setMixedNotice("");
  }, [selectedConversationID]);

  useEffect(() => {
    if (!selectedConversation) return undefined;
    const timer = window.setInterval(() => setRevokeNowMs(Date.now()), 5000);
    setRevokeNowMs(Date.now());
    return () => window.clearInterval(timer);
  }, [selectedConversationID]);

  const cleanupVoiceRecording = useCallback(() => {
    if (voiceTimerRef.current) {
      window.clearInterval(voiceTimerRef.current);
      voiceTimerRef.current = 0;
    }
    voiceStreamRef.current?.getTracks?.().forEach((track) => track.stop());
    voiceStreamRef.current = null;
    voiceRecorderRef.current = null;
    voiceChunksRef.current = [];
    voiceStartedAtRef.current = 0;
  }, []);

  useEffect(() => () => {
    voiceShouldUploadRef.current = false;
    const recorder = voiceRecorderRef.current;
    if (recorder?.state === "recording") {
      try {
        recorder.stop();
      } catch {
        // Ignore recorder shutdown errors during page teardown.
      }
    }
    cleanupVoiceRecording();
  }, [cleanupVoiceRecording]);

  const handleRealtimeGap = useCallback(() => {
    setConversationRefreshNonce((value) => value + 1);
    setMessageRefreshNonce((value) => value + 1);
  }, []);

  const fetchConversationForAISuggestion = useCallback(async (conversationID) => {
    const normalizedConversationID = String(conversationID || "").trim();
    if (!normalizedConversationID) return null;
    const pending = pendingConversationLookupsRef.current.get(normalizedConversationID);
    if (pending) return pending;

    const lookup = async (selectedAccountID) => {
      const request = buildWorkbenchConversationLookupRequest(normalizedConversationID, { selectedAccountID });
      if (!request.ok) return null;
      const payload = await requestSessionJSON("cs", request.path, { params: request.params });
      return resolveWorkbenchConversationLookupResult(payload, normalizedConversationID);
    };

    const promise = (async () => {
      const scopedAccountID = selectedAccountId || "all";
      const scoped = await lookup(scopedAccountID);
      if (scoped || scopedAccountID === "all") return scoped;
      return lookup("all");
    })().finally(() => {
      pendingConversationLookupsRef.current.delete(normalizedConversationID);
    });

    pendingConversationLookupsRef.current.set(normalizedConversationID, promise);
    return promise;
  }, [selectedAccountId]);

  const sendManagedAISuggestion = useCallback(async (suggestion) => {
    const suggestionID = String(suggestion?.suggestionId || "").trim();
    const conversationID = String(suggestion?.conversationId || "").trim();
    const text = String(suggestion?.message || "").trim();
    if (!suggestionID || !conversationID || !text) return false;
    if (pendingAISuggestionsRef.current.has(suggestionID)) return false;
    pendingAISuggestionsRef.current.add(suggestionID);

    const knownConversation = conversationByIdRef.current.get(conversationID) || {};
    let conversation = {
      ...(suggestion.conversation || {}),
      ...knownConversation,
      conversation_id: conversationID,
    };
    let payload = buildConversationReplyPayload(conversation, text, {
      aiSuggestionID: suggestionID,
      source: suggestion.source || "coze-auto-reply",
    });
    if (!payload.ok && (payload.error === "device_required" || payload.error === "receiver_required")) {
      let fetchedConversation = null;
      try {
        fetchedConversation = await fetchConversationForAISuggestion(conversationID);
      } catch {
        pendingAISuggestionsRef.current.delete(suggestionID);
        return false;
      }
      if (fetchedConversation) {
        conversation = {
          ...(suggestion.conversation || {}),
          ...fetchedConversation,
          conversation_id: conversationID,
        };
        payload = buildConversationReplyPayload(conversation, text, {
          aiSuggestionID: suggestionID,
          source: suggestion.source || "coze-auto-reply",
        });
      }
    }
    if (!payload.ok) {
      pendingAISuggestionsRef.current.delete(suggestionID);
      return false;
    }

    const localMessage = createLocalOutgoingMessage(conversation, text, {
      localID: `ai-suggestion-${suggestionID}`,
      aiSuggestionID: suggestionID,
      messageOrigin: "ai_suggestion",
      source: suggestion.source || "coze-auto-reply",
    });
    setLocalOutgoingMessages((current) => [...current, localMessage]);
    try {
      const response = await requestSessionJSON("cs", `/conversations/${encodeURIComponent(payload.conversationId)}/reply`, {
        method: "POST",
        body: payload.body,
      });
      setLocalOutgoingMessages((current) => current.map((message) => (
        message.local_id === localMessage.local_id ? reconcileLocalOutgoingMessage(message, response) : message
      )));
      setConversationRefreshNonce((value) => value + 1);
      if (selectedConversationIdRef.current === payload.conversationId) {
        setMessageRefreshNonce((value) => value + 1);
      }
      return true;
    } catch (err) {
      if (isAISuggestionConflictError(err)) {
        setLocalOutgoingMessages((current) => current.filter((message) => message.local_id !== localMessage.local_id));
        setConversationRefreshNonce((value) => value + 1);
        if (selectedConversationIdRef.current === payload.conversationId) {
          setMessageRefreshNonce((value) => value + 1);
        }
        return true;
      }
      setLocalOutgoingMessages((current) => current.map((message) => (
        message.local_id === localMessage.local_id
          ? { ...message, send_status: "failed", send_error: err.message || String(err) }
          : message
      )));
      return false;
    } finally {
      pendingAISuggestionsRef.current.delete(suggestionID);
    }
  }, [fetchConversationForAISuggestion]);

  const handleRealtimeEvent = useCallback((envelope) => {
    const aiSuggestion = resolveWorkbenchAISuggestion(envelope);
    if (aiSuggestion) {
      void sendManagedAISuggestion(aiSuggestion);
    }
    const intent = resolveWorkbenchRealtimeIntent(envelope, {
      selectedConversationId: selectedConversationIdRef.current,
    });
    if (!intent.recognized) return;
    if (intent.refreshConversations) {
      setConversationRefreshNonce((value) => value + 1);
    }
    if (intent.refreshMessages) {
      setMessageRefreshNonce((value) => value + 1);
    }
  }, [sendManagedAISuggestion]);

  const ensureRealtimeTokenFresh = useCallback(async (options) => {
    const result = await ensureSessionTokenFresh("cs", options);
    if (typeof result === "string") {
      setToken(result);
    }
    return result;
  }, []);

  const realtimeOptions = useMemo(() => ({
    getToken: () => getSessionToken("cs"),
    ensureTokenFresh: ensureRealtimeTokenFresh,
    onGap: handleRealtimeGap,
    token,
  }), [ensureRealtimeTokenFresh, handleRealtimeGap, token]);

  useRealtimeChannel(token ? "conversations" : "", workbenchConversationRealtimeTopics, handleRealtimeEvent, realtimeOptions);
  useRealtimeChannel(token ? "tasks" : "", workbenchTaskRealtimeTopics, handleRealtimeEvent, realtimeOptions);

  useEffect(() => {
    if (!token) return undefined;
    const controller = new AbortController();
    setLoading((prev) => ({ ...prev, bootstrap: true }));
    setError("");
    requestSessionJSON("cs", "/cs/workbench/bootstrap", {
      signal: controller.signal,
      params: {
        selected_account_id: selectedAccountId,
        mode_filter: modeFilter,
        status_filter: statusFilter,
        conversation_limit: conversationLimit,
      },
    })
      .then((payload) => {
        const rows = normalizeConversationRows(payload);
        setConversations(rows);
        setConversationPage(payload?.conversation_page || null);
        setSelectedConversation((current) => resolveSelectedConversation(current, rows));
      })
      .catch((err) => {
        if (err?.name !== "AbortError") {
          setConversations([]);
          setConversationPage(null);
          setError(err.message || String(err));
        }
      })
      .finally(() => {
        setLoading((prev) => ({ ...prev, bootstrap: false }));
      });
    return () => controller.abort();
  }, [conversationRefreshNonce, modeFilter, selectedAccountId, statusFilter, token]);

  useEffect(() => {
    const keyword = searchKeyword.trim();
    if (searchAbortRef.current) {
      searchAbortRef.current.abort();
      searchAbortRef.current = null;
    }
    if (!token || !keyword) {
      setSearchResults([]);
      setLoading((prev) => ({ ...prev, search: false }));
      return undefined;
    }
    const controller = new AbortController();
    searchAbortRef.current = controller;
    const timer = window.setTimeout(() => {
      setLoading((prev) => ({ ...prev, search: true }));
      requestSessionJSON("cs", "/cs/workbench/search", {
        signal: controller.signal,
        params: {
          q: keyword,
          limit: conversationLimit,
          selected_account_id: selectedAccountId,
          mode_filter: modeFilter,
          status_filter: statusFilter,
        },
      })
        .then((payload) => {
          setSearchResults(Array.isArray(payload?.results) ? payload.results : []);
        })
        .catch((err) => {
          if (err?.name !== "AbortError") {
            setSearchResults([]);
            setError(err.message || String(err));
          }
        })
        .finally(() => {
          if (searchAbortRef.current === controller) {
            searchAbortRef.current = null;
          }
          setLoading((prev) => ({ ...prev, search: false }));
        });
    }, 180);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [conversationRefreshNonce, modeFilter, searchKeyword, selectedAccountId, statusFilter, token]);

  const markConversationRead = useCallback(async (conversation) => {
    const mutation = buildConversationReadMutation(conversation);
    if (!mutation.ok) return false;
    if (markReadKeysRef.current.has(mutation.dedupeKey)) return false;
    markReadKeysRef.current.add(mutation.dedupeKey);
    try {
      const response = await requestSessionJSON("cs", mutation.path, { method: mutation.method });
      const updated = response?.conversation && typeof response.conversation === "object" ? response.conversation : {};
      const nextUnreadCount = Math.max(0, Number(updated.unread_count || 0));
      const patchConversation = (item) => {
        const itemID = String(item?.conversation_id || "").trim();
        if (itemID !== mutation.conversationId) return item;
        return { ...item, ...updated, unread_count: nextUnreadCount };
      };
      setConversations((current) => current.map(patchConversation));
      setSearchResults((current) => current.map(patchConversation));
      setSelectedConversation((current) => patchConversation(current));
      return true;
    } catch {
      markReadKeysRef.current.delete(mutation.dedupeKey);
      return false;
    }
  }, []);

  const patchConversationAIState = useCallback((conversationId, patch) => {
    const normalizedConversationId = String(conversationId || "").trim();
    if (!normalizedConversationId) return;
    const patchConversation = (item) => {
      const itemID = String(item?.conversation_id || "").trim();
      if (!item || itemID !== normalizedConversationId) return item;
      return { ...item, ...patch };
    };
    setConversations((current) => current.map(patchConversation));
    setSearchResults((current) => current.map(patchConversation));
    setSelectedConversation((current) => patchConversation(current));
  }, []);

  const handleToggleConversationAI = useCallback(async (enabled) => {
    const mutation = buildConversationAIModeMutation(selectedConversation, enabled);
    if (!mutation.ok) {
      setAIToggleError(conversationAIModeErrorMessage(mutation.error));
      return;
    }
    const conversationName = selectedConversation?.customer_name || selectedConversation?.conversation_name || selectedConversation?.sender_name || mutation.conversationId;
    if (typeof window !== "undefined" && !window.confirm(`${enabled ? "开启AI托管" : "关闭AI托管"}\n会话：${conversationName}`)) {
      return;
    }
    setAIToggleBusyConversation(mutation.conversationId);
    setAIToggleError("");
    patchConversationAIState(mutation.conversationId, {
      ai_mode_switching: true,
      ai_mode_switch_target: enabled ? "ai" : "manual",
    });
    try {
      const response = await requestSessionJSON("cs", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const returnedConversation = response?.conversation && typeof response.conversation === "object"
        ? response.conversation
        : {};
      const nextAIAutoReply = Boolean(returnedConversation.ai_auto_reply ?? response?.ai_auto_reply ?? enabled);
      const nextModeOverride = String(returnedConversation.ai_mode_override || response?.ai_mode_override || (enabled ? "auto" : "manual")).trim() || (enabled ? "auto" : "manual");
      const nextRuntimeState = returnedConversation.sop_runtime_state && typeof returnedConversation.sop_runtime_state === "object"
        ? returnedConversation.sop_runtime_state
        : {};
      const patch = {
        ...returnedConversation,
        ai_auto_reply: nextAIAutoReply,
        ai_mode_override: nextModeOverride,
        ai_mode_switching: false,
        ai_mode_switch_target: "",
        ai_reply_status: "",
        ai_reply_phase: "",
        ai_reply_force_manual: false,
        sensitive_handoff_pending: false,
        sop_runtime_state: {
          ...nextRuntimeState,
          ai_mode_switching: false,
          ai_mode_switch_target: "",
          ai_reply_status: "",
          ai_reply_phase: "",
          ai_reply_force_manual: false,
          sensitive_handoff_pending: false,
        },
      };
      if (response?.account_ai_enabled !== undefined) {
        patch.account_ai_enabled = response.account_ai_enabled;
      }
      patchConversationAIState(mutation.conversationId, patch);
      setConversationRefreshNonce((value) => value + 1);
      if (selectedConversationIdRef.current === mutation.conversationId) {
        setMessageRefreshNonce((value) => value + 1);
      }
    } catch (err) {
      patchConversationAIState(mutation.conversationId, {
        ai_mode_switching: false,
        ai_mode_switch_target: "",
      });
      setAIToggleError(err.message || String(err));
    } finally {
      setAIToggleBusyConversation((current) => (current === mutation.conversationId ? "" : current));
    }
  }, [patchConversationAIState, selectedConversation]);

  const handleDismissAIReplyError = useCallback((errorKey) => {
    rememberDismissedAIReplyError(errorKey);
    setDismissedAIReplyErrorKey(String(errorKey || "").trim());
  }, []);

  useEffect(() => {
    const conversationId = selectedConversationID;
    if (!token || !conversationId) {
      setMessages([]);
      setMessagesMeta(null);
      return undefined;
    }
    const controller = new AbortController();
    setLoading((prev) => ({ ...prev, messages: true }));
    requestSessionJSON("cs", `/conversations/${encodeURIComponent(conversationId)}/messages`, {
      signal: controller.signal,
      params: { limit: messageLimit, fresh: 1 },
    })
      .then((payload) => {
        setMessages(Array.isArray(payload?.messages) ? payload.messages : []);
        setMessagesMeta({
          total: payload?.total ?? 0,
          hasMore: Boolean(payload?.has_more),
        });
        void markConversationRead({
          conversation_id: conversationId,
          unread_count: selectedConversation?.unread_count,
          last_message_at: selectedConversation?.last_message_at,
          updated_at: selectedConversation?.updated_at,
        });
      })
      .catch((err) => {
        if (err?.name !== "AbortError") {
          setMessages([]);
          setMessagesMeta(null);
          setError(err.message || String(err));
        }
      })
      .finally(() => {
        setLoading((prev) => ({ ...prev, messages: false }));
      });
    return () => controller.abort();
  }, [markConversationRead, messageRefreshNonce, selectedConversationID, token]);

  const visibleConversations = useMemo(() => {
    return searchKeyword.trim() ? searchResults : conversations;
  }, [conversations, searchKeyword, searchResults]);

  const displayedMessages = useMemo(() => {
    return mergeLocalOutgoingMessages(messages, localOutgoingMessages, selectedConversation?.conversation_id || "");
  }, [localOutgoingMessages, messages, selectedConversation?.conversation_id]);
  const activeAISuggestionEdit = aiSuggestionEdit?.conversationId === selectedConversation?.conversation_id ? aiSuggestionEdit : null;

  const handleLogin = useCallback(async (event) => {
    event.preventDefault();
    if (!loginAssigneeId.trim() || !loginPassword.trim()) {
      setAuthError("请输入账号和密码");
      return;
    }
    setAuthLoading(true);
    setAuthError("");
    try {
      const response = await loginCSWithPassword(loginAssigneeId, loginPassword);
      setToken(response.token);
      setLoginAssigneeId(response.assignee_id || loginAssigneeId.trim());
      setLoginPassword("");
      setError("");
      setConversationRefreshNonce((value) => value + 1);
    } catch (err) {
      setAuthError(sessionLoginErrorMessage("cs", err));
    } finally {
      setAuthLoading(false);
    }
  }, [loginAssigneeId, loginPassword]);

  const handleLogout = useCallback(async () => {
    const previousToken = token;
    setToken("");
    setConversations([]);
    setSearchResults([]);
    setConversationPage(null);
    setSelectedConversation(null);
    setMessages([]);
    setMessagesMeta(null);
    setLocalOutgoingMessages([]);
    setError("");
    try {
      await logoutSession("cs", { token: previousToken });
    } catch {
      // Logout is local-first; stale or already revoked tokens should not block leaving the workspace.
    }
  }, [token]);

  const sendEditedAISuggestion = useCallback(async (edit, text) => {
    const suggestionID = String(edit?.suggestionId || "").trim();
    const conversationID = String(edit?.conversationId || "").trim();
    const localID = String(edit?.localId || "").trim();
    const message = String(text || "").trim();
    if (!selectedConversation || selectedConversation.conversation_id !== conversationID || !suggestionID || !message) return false;
    if (pendingAISuggestionsRef.current.has(suggestionID)) return false;

    const payload = buildConversationReplyPayload(selectedConversation, message, {
      aiSuggestionID: suggestionID,
      source: edit.source || "coze-auto-reply",
    });
    if (!payload.ok) {
      const messagesByError = {
        conversation_required: "请选择会话",
        message_required: "请输入消息内容",
        device_required: "当前会话缺少可用设备",
        sender_required: "当前会话缺少发送目标",
      };
      setError(messagesByError[payload.error] || "当前会话暂不可发送");
      return false;
    }

    pendingAISuggestionsRef.current.add(suggestionID);
    setSending(true);
    setError("");
    setLocalOutgoingMessages((current) => current.map((item) => (
      item.local_id === localID || item.trace_id === localID
        ? { ...item, content: message, send_status: "pending", send_error: "", ai_suggestion_edited: true }
        : item
    )));
    try {
      const response = await requestSessionJSON("cs", `/conversations/${encodeURIComponent(payload.conversationId)}/reply`, {
        method: "POST",
        body: payload.body,
      });
      setLocalOutgoingMessages((current) => current.map((item) => (
        item.local_id === localID || item.trace_id === localID ? reconcileLocalOutgoingMessage(item, response) : item
      )));
      setAISuggestionEdit(null);
      setSendDraft("");
      setConversationRefreshNonce((value) => value + 1);
      setMessageRefreshNonce((value) => value + 1);
      return true;
    } catch (err) {
      if (isAISuggestionConflictError(err)) {
        setLocalOutgoingMessages((current) => current.filter((item) => item.local_id !== localID && item.trace_id !== localID));
        setAISuggestionEdit(null);
        setSendDraft("");
        setConversationRefreshNonce((value) => value + 1);
        setMessageRefreshNonce((value) => value + 1);
        return true;
      }
      const reason = err.message || String(err);
      setError(reason);
      setLocalOutgoingMessages((current) => current.map((item) => (
        item.local_id === localID || item.trace_id === localID
          ? { ...item, content: message, send_status: "failed", send_error: reason, ai_suggestion_edited: true }
          : item
      )));
      return false;
    } finally {
      pendingAISuggestionsRef.current.delete(suggestionID);
      setSending(false);
    }
  }, [selectedConversation]);

  const handleSend = useCallback(async () => {
    if (sending) return;
    const text = sendDraft.trim();
    const conversation = selectedConversation;
    if (
      aiSuggestionEdit?.suggestionId &&
      aiSuggestionEdit.conversationId === conversation?.conversation_id
    ) {
      await sendEditedAISuggestion(aiSuggestionEdit, text);
      return;
    }
    const batch = nextManualTextClientBatch(manualTextClientBatchRef.current, conversation);
    manualTextClientBatchRef.current = batch.state;
    const payload = buildConversationReplyPayload(conversation, text, { clientBatch: batch.payload });
    if (!payload.ok) {
      const messagesByError = {
        conversation_required: "请选择会话",
        message_required: "请输入消息内容",
        device_required: "当前会话缺少可用设备",
        sender_required: "当前会话缺少发送目标",
      };
      setError(messagesByError[payload.error] || "当前会话暂不可发送");
      return;
    }

    const localMessage = createLocalOutgoingMessage(conversation, text);
    setLocalOutgoingMessages((current) => [...current, localMessage]);
    setSendDraft("");
    setSending(true);
    setError("");
    try {
      const response = await requestSessionJSON("cs", `/conversations/${encodeURIComponent(payload.conversationId)}/reply`, {
        method: "POST",
        body: payload.body,
      });
      setLocalOutgoingMessages((current) => current.map((message) => (
        message.local_id === localMessage.local_id ? reconcileLocalOutgoingMessage(message, response) : message
      )));
      setConversationRefreshNonce((value) => value + 1);
      setMessageRefreshNonce((value) => value + 1);
    } catch (err) {
      setLocalOutgoingMessages((current) => current.map((message) => (
        message.local_id === localMessage.local_id
          ? { ...message, send_status: "failed", send_error: err.message || String(err) }
          : message
      )));
      setError(err.message || String(err));
    } finally {
      setSending(false);
    }
  }, [aiSuggestionEdit, selectedConversation, sendDraft, sendEditedAISuggestion, sending]);

  const handleEditAISuggestion = useCallback((message) => {
    const edit = buildAISuggestionEditDraft(message);
    if (!edit) return;
    setAISuggestionEdit(edit);
    setSendDraft(edit.text);
    setError("");
  }, []);

  const cancelAISuggestionEdit = useCallback(() => {
    setAISuggestionEdit(null);
    setSendDraft("");
  }, []);

  const uploadConversationMediaFile = useCallback(async (file, options = {}) => {
    if (sending) {
      return false;
    }
    if (!file) return false;
    const conversation = selectedConversation;
    const payload = buildConversationMediaSendPayload(conversation, file, {
      kind: options.kind,
      voiceDurationSec: options.voiceDurationSec ?? file.voiceDurationSec,
    });
    if (!payload.ok) {
      const messagesByError = {
        conversation_required: "请选择会话",
        file_required: "请选择文件",
        file_too_large: "文件不能超过 50MB",
        device_required: "当前会话缺少可用设备",
        sender_required: "当前会话缺少发送目标",
        media_kind_required: "不支持的文件类型",
        formdata_unavailable: "当前浏览器不支持文件上传",
      };
      setError(messagesByError[payload.error] || "当前文件暂不可发送");
      return false;
    }

    const retrySource = options.localMessage && typeof options.localMessage === "object" ? options.localMessage : null;
    const localMessage = retrySource
      ? createLocalMediaRetryMessage(retrySource)
      : createLocalMediaOutgoingMessage(conversation, file, payload.kind, {
        voiceDurationSec: options.voiceDurationSec ?? file.voiceDurationSec,
      });
    setLocalOutgoingMessages((current) => (
      retrySource
        ? current.map((message) => (message.local_id === localMessage.local_id ? localMessage : message))
        : [...current, localMessage]
    ));
    setSending(true);
    setError("");
    try {
      const response = await requestSessionJSON("cs", payload.endpoint, {
        method: "POST",
        body: payload.formData,
      });
      setLocalOutgoingMessages((current) => current.map((message) => (
        message.local_id === localMessage.local_id ? reconcileLocalOutgoingMessage(message, response) : message
      )));
      setConversationRefreshNonce((value) => value + 1);
      setMessageRefreshNonce((value) => value + 1);
    } catch (err) {
      setLocalOutgoingMessages((current) => current.map((message) => (
        message.local_id === localMessage.local_id
          ? { ...message, send_status: "failed", send_error: err.message || String(err) }
          : message
      )));
      setError(err.message || String(err));
      return false;
    } finally {
      setSending(false);
    }
    return true;
  }, [selectedConversation, sending]);

  const retryLocalMediaMessage = useCallback(async (message) => {
    if (sending || !canRetryLocalMediaMessage(message)) return;
    await uploadConversationMediaFile(message.local_media_file, {
      kind: message.local_media_kind || message.msg_type,
      voiceDurationSec: message.voice_duration_sec,
      localMessage: message,
    });
  }, [sending, uploadConversationMediaFile]);

  const handlePrepareArchiveMedia = useCallback(async (taskId) => {
    const normalizedTaskID = String(taskId || "").trim();
    if (!normalizedTaskID) return;
    setError("");
    try {
      await requestSessionJSON("cs", `/archive/media/tasks/${encodeURIComponent(normalizedTaskID)}/prepare`, {
        method: "POST",
      });
      setConversationRefreshNonce((value) => value + 1);
      setMessageRefreshNonce((value) => value + 1);
    } catch (err) {
      const reason = err.message || String(err);
      setError(reason);
      throw err;
    }
  }, []);

  const handleRetryVoiceTranscription = useCallback(async (payload) => {
    setError("");
    try {
      const response = await requestSessionJSON("cs", "/archive/voice-transcriptions/retry", {
        method: "POST",
        body: payload || {},
      });
      setConversationRefreshNonce((value) => value + 1);
      setMessageRefreshNonce((value) => value + 1);
      return response;
    } catch (err) {
      const reason = err.message || String(err);
      setError(reason);
      throw err;
    }
  }, []);

  const handleMediaFileChange = useCallback(async (event) => {
    if (sending) {
      if (event?.target) event.target.value = "";
      return;
    }
    const file = event?.target?.files?.[0] || null;
    if (event?.target) event.target.value = "";
    if (!file) return;
    await uploadConversationMediaFile(file);
  }, [sending, uploadConversationMediaFile]);

  const stopVoiceRecording = useCallback(() => {
    const recorder = voiceRecorderRef.current;
    if (!recorder || recorder.state === "inactive") return;
    try {
      recorder.stop();
    } catch (err) {
      setVoiceRecordingError(err.message || String(err));
      cleanupVoiceRecording();
      setVoiceRecording(false);
      setVoiceRecordingSeconds(0);
    }
  }, [cleanupVoiceRecording]);

  const cancelVoiceRecording = useCallback(() => {
    voiceShouldUploadRef.current = false;
    const recorder = voiceRecorderRef.current;
    if (!recorder || recorder.state === "inactive") {
      cleanupVoiceRecording();
      setVoiceRecording(false);
      setVoiceRecordingSeconds(0);
      setVoiceRecordingError("");
      return;
    }
    try {
      recorder.stop();
    } catch {
      cleanupVoiceRecording();
      setVoiceRecording(false);
      setVoiceRecordingSeconds(0);
      setVoiceRecordingError("");
    }
  }, [cleanupVoiceRecording]);

  const startVoiceRecording = useCallback(async () => {
    if (sending || voiceRecording) return;
    if (typeof navigator === "undefined" || !navigator.mediaDevices?.getUserMedia) {
      setVoiceRecordingError("当前浏览器不支持录音");
      return;
    }
    if (typeof MediaRecorder === "undefined") {
      setVoiceRecordingError("当前浏览器不支持录音");
      return;
    }

    setVoiceRecordingError("");
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: { echoCancellation: true, noiseSuppression: true },
        video: false,
      });
      const mimeType = selectVoiceRecorderMimeType(MediaRecorder);
      const recorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined);
      voiceRecorderRef.current = recorder;
      voiceStreamRef.current = stream;
      voiceChunksRef.current = [];
      voiceStartedAtRef.current = Date.now();
      voiceShouldUploadRef.current = true;

      recorder.ondataavailable = (event) => {
        if (event?.data?.size > 0) {
          voiceChunksRef.current.push(event.data);
        }
      };
      recorder.onstop = () => {
        const chunks = voiceChunksRef.current.slice();
        const startedAt = voiceStartedAtRef.current || Date.now();
        const uploadRequested = voiceShouldUploadRef.current;
        const recordedSec = Math.max(1, Math.round((Date.now() - startedAt) / 1000));
        const recordedMimeType = recorder.mimeType || mimeType || "audio/webm";
        cleanupVoiceRecording();
        setVoiceRecording(false);
        setVoiceRecordingSeconds(0);
        if (!uploadRequested) return;
        if (!shouldUploadVoiceRecording({ shouldUpload: uploadRequested, chunks })) {
          setVoiceRecordingError("录音内容为空");
          return;
        }
        let file;
        try {
          file = createVoiceRecordingFile(chunks, {
            mimeType: recordedMimeType,
            durationSec: recordedSec,
          });
        } catch (err) {
          setVoiceRecordingError(err.message || String(err));
          return;
        }
        void uploadConversationMediaFile(file, { kind: "voice", voiceDurationSec: file.voiceDurationSec });
      };
      recorder.onerror = (event) => {
        setVoiceRecordingError(voiceRecordingErrorMessage(event?.error));
      };

      recorder.start(1000);
      setVoiceRecording(true);
      setVoiceRecordingSeconds(0);
      voiceTimerRef.current = window.setInterval(() => {
        const elapsedMs = Date.now() - (voiceStartedAtRef.current || Date.now());
        const elapsedSec = Math.floor(elapsedMs / 1000);
        setVoiceRecordingSeconds(elapsedSec);
        if (elapsedMs >= VOICE_RECORDING_MAX_MS) {
          stopVoiceRecording();
        }
      }, 250);
    } catch (err) {
      cleanupVoiceRecording();
      setVoiceRecording(false);
      setVoiceRecordingSeconds(0);
      setVoiceRecordingError(voiceRecordingErrorMessage(err));
    }
  }, [cleanupVoiceRecording, sending, stopVoiceRecording, uploadConversationMediaFile, voiceRecording]);

  const handleVoiceRecordingClick = useCallback(() => {
    if (voiceRecording) {
      stopVoiceRecording();
      return;
    }
    void startVoiceRecording();
  }, [startVoiceRecording, stopVoiceRecording, voiceRecording]);

  const handleStartCall = useCallback(async (callType) => {
    if (callBusy) return;
    const payload = buildConversationCallPayload(selectedConversation, callType, {
      reservationID: callState?.conversationId === selectedConversation?.conversation_id ? callState?.reservationId : "",
    });
    if (!payload.ok) {
      const messagesByError = {
        conversation_required: "请选择会话",
        device_required: "当前会话缺少可用设备",
        call_type_required: "不支持的通话类型",
      };
      setError(messagesByError[payload.error] || "当前会话暂不可拨打");
      return;
    }

    setCallBusy(payload.callType);
    setError("");
    try {
      const response = await requestSessionJSON("cs", `/conversations/${encodeURIComponent(payload.conversationId)}/call`, {
        method: "POST",
        body: payload.body,
      });
      setCallState({
        conversationId: payload.conversationId,
        callType: payload.callType,
        reservationId: String(response?.reservation_id || "").trim(),
        taskId: String(response?.task?.task_id || "").trim(),
        status: "queued",
      });
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setCallBusy("");
    }
  }, [callBusy, callState?.conversationId, callState?.reservationId, selectedConversation]);

  const handleHangupCall = useCallback(async () => {
    if (callBusy) return;
    const payload = buildConversationHangupPayload(selectedConversation, {
      reservationID: callState?.conversationId === selectedConversation?.conversation_id ? callState?.reservationId : "",
    });
    if (!payload.ok) {
      const messagesByError = {
        conversation_required: "请选择会话",
        device_required: "当前会话缺少可用设备",
      };
      setError(messagesByError[payload.error] || "当前会话暂不可挂断");
      return;
    }

    setCallBusy("hangup");
    setError("");
    try {
      const response = await requestSessionJSON("cs", `/conversations/${encodeURIComponent(payload.conversationId)}/call/hangup`, {
        method: "POST",
        body: payload.body,
      });
      setCallState({
        conversationId: payload.conversationId,
        callType: callState?.callType || "voice",
        reservationId: "",
        taskId: String(response?.task?.task_id || "").trim(),
        status: "hangup_queued",
      });
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setCallBusy("");
    }
  }, [callBusy, callState?.callType, callState?.conversationId, callState?.reservationId, selectedConversation]);

  const patchMessageByTrace = useCallback((traceId, updater) => {
    const targetTraceID = String(traceId || "").trim();
    if (!targetTraceID || typeof updater !== "function") return;
    const patchRows = (rows) => rows.map((message) => (
      String(message?.trace_id || "").trim() === targetTraceID ? updater(message) : message
    ));
    setMessages((current) => patchRows(current));
    setLocalOutgoingMessages((current) => patchRows(current));
  }, []);

  const handleResend = useCallback(async (message) => {
    const payload = buildConversationResendPayload(selectedConversation, message);
    if (!payload.ok) {
      const messagesByError = {
        conversation_required: "请选择会话",
        trace_required: "该消息缺少补发标识",
        message_not_resendable: "该消息当前不可补发",
      };
      setError(messagesByError[payload.error] || "该消息当前不可补发");
      return;
    }

    patchMessageByTrace(payload.traceId, (item) => ({ ...item, resend_status: "pending", resend_error: "" }));
    setError("");
    try {
      const response = await requestSessionJSON(
        "cs",
        `/conversations/${encodeURIComponent(payload.conversationId)}/messages/${encodeURIComponent(payload.traceId)}/resend`,
        {
          method: "POST",
          body: payload.body,
        },
      );
      const resentMessage = createResendOutgoingMessage(response);
      const responseStatus = String(resentMessage?.send_status || response?.task?.status || "pending").trim().toLowerCase() || "pending";
      const resendStatus = responseStatus === "success" ? "success" : responseStatus === "failed" ? "failed" : "queued";
      patchMessageByTrace(payload.traceId, (item) => ({
        ...item,
        resend_status: resendStatus,
        resend_error: resendStatus === "failed" ? (resentMessage?.send_error || "补发任务创建失败") : "",
        resend_task_id: String(response?.task?.task_id || resentMessage?.task_id || "").trim(),
        resend_trace_id: String(resentMessage?.trace_id || "").trim(),
      }));
      if (resentMessage) {
        setLocalOutgoingMessages((current) => [...current, resentMessage]);
      }
      setConversationRefreshNonce((value) => value + 1);
      setMessageRefreshNonce((value) => value + 1);
    } catch (err) {
      const reason = err.message || String(err);
      patchMessageByTrace(payload.traceId, (item) => ({ ...item, resend_status: "failed", resend_error: reason }));
      setError(reason);
    }
  }, [patchMessageByTrace, selectedConversation]);

  const handleRevoke = useCallback(async (message) => {
    const occurrenceFromBottom = resolveRevokeOccurrenceFromBottom(displayedMessages, message);
    const payload = buildConversationRevokePayload(selectedConversation, message, {
      nowMs: Date.now(),
      occurrenceFromBottom,
    });
    if (!payload.ok) {
      const messagesByError = {
        conversation_required: "请选择会话",
        trace_required: "该消息缺少撤回标识",
        device_required: "当前会话缺少可用设备",
        message_not_revocable: "该消息当前不可撤回",
      };
      setError(messagesByError[payload.error] || "该消息当前不可撤回");
      return;
    }

    patchMessageByTrace(payload.traceId, (item) => ({ ...item, revoke_status: "pending", revoke_error: "" }));
    setError("");
    try {
      const response = await requestSessionJSON(
        "cs",
        `/conversations/${encodeURIComponent(payload.conversationId)}/messages/${encodeURIComponent(payload.traceId)}/revoke`,
        {
          method: "POST",
          body: payload.body,
        },
      );
      const responseMessage = response?.message && typeof response.message === "object" ? response.message : {};
      patchMessageByTrace(payload.traceId, (item) => ({
        ...item,
        ...responseMessage,
        revoke_status: String(responseMessage.revoke_status || "pending").trim() || "pending",
        revoke_task_id: String(responseMessage.revoke_task_id || response?.task?.task_id || "").trim(),
        revoke_error: String(responseMessage.revoke_error || "").trim(),
      }));
      setConversationRefreshNonce((value) => value + 1);
      setMessageRefreshNonce((value) => value + 1);
    } catch (err) {
      const reason = err.message || String(err);
      patchMessageByTrace(payload.traceId, (item) => ({ ...item, revoke_status: "failed", revoke_error: reason }));
      setError(reason);
    }
  }, [displayedMessages, patchMessageByTrace, selectedConversation]);

  const updateMixedMessage = useCallback((id, field, value) => {
    setMixedMessages((current) => current.map((item) => (
      item.id === id ? { ...item, [field]: value } : item
    )));
  }, []);

  const addMixedMessage = useCallback(() => {
    setMixedMessages((current) => [...current, createMixedMessageDraft(`mixed-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`)]);
  }, []);

  const removeMixedMessage = useCallback((id) => {
    setMixedMessages((current) => {
      if (current.length <= 1) return [createMixedMessageDraft()];
      return current.filter((item) => item.id !== id);
    });
  }, []);

  const handleSidebarMixedSend = useCallback(async () => {
    if (mixedSending || sending || voiceRecording) return;
    const mutation = buildSidebarMixedMessagesMutation(selectedConversation, mixedMessages);
    if (!mutation.ok) {
      setMixedNotice(sidebarMixedMessageErrorMessage(mutation.error));
      return;
    }

    setMixedSending(true);
    setMixedNotice("");
    try {
      const response = await requestSessionJSON("cs", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const taskId = String(response?.msg_id || response?.task?.task_id || "").trim();
      setMixedNotice(taskId ? `聚合消息已提交：${taskId}` : "聚合消息已提交");
      setMixedMessages([createMixedMessageDraft()]);
      setConversationRefreshNonce((value) => value + 1);
      setMessageRefreshNonce((value) => value + 1);
    } catch (err) {
      setMixedNotice(err.message || String(err));
    } finally {
      setMixedSending(false);
    }
  }, [mixedMessages, mixedSending, selectedConversation, sending, voiceRecording]);

  const handleComposerKeyDown = (event) => {
    if (event.key !== "Enter" || event.shiftKey) return;
    event.preventDefault();
    void handleSend();
  };

  if (!token) {
    return (
      <WorkbenchLoginPanel
        assigneeId={loginAssigneeId}
        password={loginPassword}
        loading={authLoading}
        error={authError}
        onAssigneeIdChange={setLoginAssigneeId}
        onPasswordChange={setLoginPassword}
        onSubmit={handleLogin}
      />
    );
  }

  return (
    <div className="mx-auto grid max-w-7xl grid-rows-[auto_1fr] gap-4 px-4 py-4 lg:h-[calc(100vh-49px)] lg:px-6">
      <section className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(220px,1fr)_auto_auto_auto_auto]">
        <div className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">客服会话</span>
          <span className="h-9 truncate border border-[#e5e9f2] bg-[#f9fafc] px-2 py-2 text-sm text-[#172033]">
            {loginAssigneeId || "已连接"}
          </span>
        </div>
        <FilterSelect label="账号" value={selectedAccountId} onChange={setSelectedAccountId} options={["all", "assigned-sessions"]} />
        <FilterSelect label="模式" value={modeFilter} onChange={setModeFilter} options={["all", "manual", "ai", "sensitive"]} />
        <FilterSelect label="状态" value={statusFilter} onChange={setStatusFilter} options={["pending", "all", "unread", "replied"]} />
        <div className="flex items-end gap-2">
          <button className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]" type="button" onClick={() => setConversationRefreshNonce((value) => value + 1)}>
            刷新
          </button>
          <button className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white" type="button" onClick={() => void handleLogout()}>
            退出
          </button>
        </div>
      </section>

      <section className="grid min-h-[620px] gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
        <aside className="grid min-h-0 grid-rows-[auto_1fr] border border-[#d8dde8] bg-white">
          <div className="border-b border-[#e5e9f2] p-3">
            <input
              className="h-9 w-full border border-[#cfd6e3] px-2 text-sm outline-none focus:border-[#2f6fed]"
              value={searchKeyword}
              onChange={(event) => setSearchKeyword(event.target.value)}
              placeholder="搜索联系人或会话"
            />
            <div className="mt-2 flex items-center justify-between text-xs text-[#697386]">
              <span>{searchKeyword.trim() ? "搜索结果" : "会话队列"}</span>
              <span>{conversationPage?.returned ?? visibleConversations.length} / {conversationPage?.total ?? visibleConversations.length}</span>
            </div>
          </div>
          <div className="min-h-0 overflow-y-auto">
            {(loading.bootstrap || loading.search) && <ConversationSkeleton />}
            {!loading.bootstrap && !loading.search && visibleConversations.length === 0 && (
              <EmptyState label={token ? "暂无会话" : "等待会话 Token"} />
            )}
            {!loading.bootstrap && !loading.search && visibleConversations.map((conversation) => (
              <ConversationRow
                key={conversation.conversation_id || conversation.conversation_key}
                conversation={conversation}
                selected={conversation.conversation_id === selectedConversation?.conversation_id}
                onSelect={() => setSelectedConversation(conversation)}
              />
            ))}
          </div>
        </aside>

        <main className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)_auto] border border-[#d8dde8] bg-white">
          <ConversationHeader
            conversation={selectedConversation}
            error={error}
            aiToggleBusy={Boolean(aiToggleBusyConversation && aiToggleBusyConversation === selectedConversationID)}
            aiToggleError={aiToggleError}
            dismissedAIReplyErrorKey={dismissedAIReplyErrorKey}
            onToggleAI={handleToggleConversationAI}
            onDismissAIReplyError={handleDismissAIReplyError}
            callBusy={callBusy}
            callState={callState}
            onStartCall={handleStartCall}
            onHangupCall={handleHangupCall}
          />
          <div className="min-h-0 overflow-y-auto bg-[#f9fafc] p-4">
            {loading.messages && <MessageSkeleton />}
            {!loading.messages && !selectedConversation && <EmptyState label="选择会话" />}
            {!loading.messages && selectedConversation && displayedMessages.length === 0 && <EmptyState label="暂无消息" />}
            {!loading.messages && displayedMessages.map((message) => (
              <MessageRow
                key={message.trace_id || message.message_id}
                message={message}
                onResend={handleResend}
                onRevoke={handleRevoke}
                onRetryLocalMedia={retryLocalMediaMessage}
                onEditAISuggestion={handleEditAISuggestion}
                onPrepareArchiveMedia={handlePrepareArchiveMedia}
                onRetryVoiceTranscription={handleRetryVoiceTranscription}
                onPreviewMedia={setPreviewMedia}
                revokeNowMs={revokeNowMs}
              />
            ))}
          </div>
          {selectedConversation && (
            <div className="grid gap-2 border-t border-[#e5e9f2] bg-white px-4 py-3">
              <div className="flex items-center justify-between text-xs text-[#697386]">
                <span>
                  {activeAISuggestionEdit
                    ? "正在编辑 AI 建议"
                    : `已读取 ${messages.length} 条${messagesMeta?.hasMore ? "，仍有更早消息" : ""}`}
                </span>
                <span className={voiceRecordingError ? "text-[#b42318]" : ""}>
                  {voiceRecording ? `录音 ${formatVoiceRecordingSeconds(voiceRecordingSeconds)}` : sending ? "发送中" : voiceRecordingError || " "}
                </span>
              </div>
              <div className={voiceRecording ? "grid gap-2 sm:grid-cols-[auto_auto_auto_minmax(0,1fr)_auto]" : "grid gap-2 sm:grid-cols-[auto_auto_minmax(0,1fr)_auto]"}>
                <input
                  ref={mediaInputRef}
                  className="hidden"
                  type="file"
                  accept="image/*,video/*,audio/*,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.csv,.zip"
                  onChange={(event) => void handleMediaFileChange(event)}
                />
                <button
                  className="h-10 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                  type="button"
                  disabled={!token || sending || voiceRecording}
                  onClick={() => mediaInputRef.current?.click()}
                >
                  上传
                </button>
                <button
                  className={`h-10 border px-3 text-sm font-medium disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386] ${
                    voiceRecording
                      ? "border-[#b42318] bg-[#fff4f2] text-[#b42318]"
                      : "border-[#cfd6e3] bg-white text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed]"
                  }`}
                  type="button"
                  disabled={!token || (sending && !voiceRecording)}
                  onClick={handleVoiceRecordingClick}
                >
                  {voiceRecording ? "停止" : "录音"}
                </button>
                {voiceRecording && (
                  <button
                    className="h-10 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#697386] hover:border-[#b42318] hover:text-[#b42318]"
                    type="button"
                    disabled={!token}
                    onClick={cancelVoiceRecording}
                  >
                    取消
                  </button>
                )}
                <textarea
                  className="min-h-20 resize-none border border-[#cfd6e3] px-3 py-2 text-sm leading-6 outline-none focus:border-[#2f6fed]"
                  value={sendDraft}
                  onChange={(event) => setSendDraft(event.target.value)}
                  onKeyDown={handleComposerKeyDown}
                  placeholder="输入回复内容"
                />
                <button
                  className="h-10 border border-[#172033] bg-[#172033] px-4 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
                  type="button"
                  disabled={!token || !sendDraft.trim() || sending || voiceRecording}
                  onClick={() => void handleSend()}
                >
                  {activeAISuggestionEdit ? "发送修改" : "发送"}
                </button>
              </div>
              {activeAISuggestionEdit && (
                <div className="flex justify-end">
                  <button
                    className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#697386] hover:border-[#2f6fed] hover:text-[#2f6fed]"
                    type="button"
                    disabled={sending}
                    onClick={cancelAISuggestionEdit}
                  >
                    取消编辑
                  </button>
                </div>
              )}
              <div className="grid gap-2 border-t border-[#edf0f5] pt-2">
                <div className="flex items-center justify-between gap-2 text-xs text-[#697386]">
                  <span>聚合消息</span>
                  <button
                    className="h-8 border border-[#cfd6e3] bg-white px-2 font-medium text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                    type="button"
                    disabled={!token || mixedSending || sending || voiceRecording}
                    onClick={addMixedMessage}
                  >
                    新增
                  </button>
                </div>
                {mixedMessages.map((item) => (
                  <div key={item.id} className="grid gap-2 sm:grid-cols-[96px_minmax(0,1fr)_auto]">
                    <select
                      className="h-9 border border-[#cfd6e3] bg-white px-2 text-sm outline-none focus:border-[#2f6fed]"
                      value={item.type}
                      disabled={mixedSending || sending || voiceRecording}
                      onChange={(event) => updateMixedMessage(item.id, "type", event.target.value)}
                    >
                      {mixedMessageTypeOptions.map((option) => (
                        <option key={option.value} value={option.value}>{option.label}</option>
                      ))}
                    </select>
                    <input
                      className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                      value={item.content}
                      disabled={mixedSending || sending || voiceRecording}
                      onChange={(event) => updateMixedMessage(item.id, "content", event.target.value)}
                      placeholder="文本或 URL"
                    />
                    <button
                      className="h-9 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#697386] hover:border-[#b42318] hover:text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={mixedSending || sending || voiceRecording}
                      onClick={() => removeMixedMessage(item.id)}
                    >
                      删除
                    </button>
                  </div>
                ))}
                <div className="flex items-center justify-between gap-2 text-xs">
                  <span className={mixedNotice.includes("失败") || mixedNotice.includes("缺少") || mixedNotice.includes("请输入") || mixedNotice.includes("无法") ? "text-[#b42318]" : "text-[#697386]"}>
                    {mixedNotice || " "}
                  </span>
                  <button
                    className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
                    type="button"
                    disabled={!token || !selectedConversation || mixedSending || sending || voiceRecording}
                    onClick={() => void handleSidebarMixedSend()}
                  >
                    {mixedSending ? "提交中" : "发送聚合"}
                  </button>
                </div>
              </div>
            </div>
          )}
        </main>
      </section>
      <MediaPreviewDialog preview={previewMedia} onClose={() => setPreviewMedia(null)} />
    </div>
  );
}

function WorkbenchLoginPanel({ assigneeId, password, loading, error, onAssigneeIdChange, onPasswordChange, onSubmit }) {
  return (
    <div className="mx-auto grid max-w-7xl px-4 py-4 lg:px-6">
      <section className="grid min-h-[620px] items-center border border-[#d8dde8] bg-white p-4 md:p-8">
        <form className="mx-auto grid w-full max-w-sm gap-4" onSubmit={onSubmit}>
          <div>
            <h1 className="text-lg font-semibold text-[#172033]">客服工作台登录</h1>
            <p className="mt-1 text-xs text-[#697386]">/api/v1/session/cs-login</p>
          </div>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">客服账号</span>
            <input
              className="h-10 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={assigneeId}
              onChange={(event) => onAssigneeIdChange(event.target.value)}
              placeholder="assignee_id"
              autoComplete="username"
              autoFocus
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">密码</span>
            <input
              className="h-10 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              type="password"
              value={password}
              onChange={(event) => onPasswordChange(event.target.value)}
              placeholder="password"
              autoComplete="current-password"
            />
          </label>
          {error && <div className="border border-[#f2b8b5] bg-[#fff4f2] px-3 py-2 text-sm text-[#b42318]">{error}</div>}
          <button
            className="h-10 border border-[#172033] bg-[#172033] px-4 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={loading}
          >
            {loading ? "登录中" : "登录"}
          </button>
        </form>
      </section>
    </div>
  );
}

function normalizeConversationRows(payload) {
  if (Array.isArray(payload?.conversations)) return payload.conversations;
  const layers = payload?.conversation_layers || {};
  return [...(layers.hot || []), ...(layers.warm || []), ...(layers.cold || [])];
}

function resolveSelectedConversation(current, rows) {
  if (!Array.isArray(rows) || rows.length === 0) return null;
  if (!current) return rows[0];
  return rows.find((row) => row.conversation_id === current.conversation_id) || rows[0];
}

function FilterSelect({ label, value, onChange, options }) {
  return (
    <label className="grid gap-1">
      <span className="text-xs font-medium text-[#697386]">{label}</span>
      <select className="h-9 border border-[#cfd6e3] bg-white px-2 text-sm outline-none focus:border-[#2f6fed]" value={value} onChange={(event) => onChange(event.target.value)}>
        {options.map((option) => (
          <option key={option} value={option}>{option}</option>
        ))}
      </select>
    </label>
  );
}

function ConversationRow({ conversation, selected, onSelect }) {
  const title = conversation.customer_name || conversation.conversation_name || conversation.sender_name || conversation.sender_id || "未命名会话";
  const badges = resolveWorkbenchConversationBadges(conversation);
  const replyClassName = badges.status.replyState === "pending"
    ? (badges.status.isOverdue ? "bg-[#fff4f2] text-[#b42318]" : "bg-[#fff7e6] text-[#9a6700]")
    : "bg-[#ecfdf3] text-[#067647]";
  const modeClassName = badges.status.sensitiveHandoffPending
    ? "bg-[#fff1f3] text-[#c01048]"
    : badges.status.modeState === "ai"
      ? "bg-[#eef4ff] text-[#175cd3]"
      : "bg-[#fff7ed] text-[#b54708]";
  return (
    <button className={`block w-full border-b border-[#edf0f5] px-3 py-3 text-left hover:bg-[#f5f7fb] ${selected ? "bg-[#eef4ff]" : "bg-white"}`} type="button" onClick={onSelect}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <span className="truncate text-sm font-semibold text-[#172033]">{title}</span>
        <span className="shrink-0 text-xs text-[#697386]">{formatTime(conversation.last_message_at)}</span>
      </div>
      <div className="mt-1 truncate text-xs text-[#566072]">{conversation.last_content || conversation.account_name || conversation.search_kind || ""}</div>
      <div className="mt-2 flex items-center gap-2 text-xs text-[#697386]">
        <span>{conversation.account_name || conversation.account_wework_user_id || conversation.device_id || "未绑定账号"}</span>
        {Number(conversation.unread_count || 0) > 0 && <span className="bg-[#d92d20] px-1.5 py-0.5 text-white">{conversation.unread_count}</span>}
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5 text-[11px]">
        <span className={`px-1.5 py-0.5 ${replyClassName}`}>{badges.replyLabel}</span>
        <span className={`px-1.5 py-0.5 ${modeClassName}`} title={badges.modeTitle}>{badges.modeLabel}</span>
        {badges.runtimeLabel && <span className="bg-[#eef4ff] px-1.5 py-0.5 text-[#175cd3]">{badges.runtimeLabel}</span>}
      </div>
    </button>
  );
}

function ConversationHeader({
  conversation,
  error,
  aiToggleBusy,
  aiToggleError,
  dismissedAIReplyErrorKey,
  onToggleAI,
  onDismissAIReplyError,
  callBusy,
  callState,
  onStartCall,
  onHangupCall,
}) {
  const title = conversation?.customer_name || conversation?.conversation_name || conversation?.sender_name || "未选择会话";
  const callStatus = callStatusText(callState, conversation?.conversation_id);
  const callDisabled = !conversation || Boolean(callBusy);
  const badges = conversation ? resolveWorkbenchConversationBadges(conversation) : null;
  const aiReplyErrorNotice = conversation
    ? resolveWorkbenchAIReplyErrorNotice(conversation, {
      dismissedKey: dismissedAIReplyErrorKey,
      isDismissed: isAIReplyErrorDismissed,
    })
    : null;
  const aiToggleState = conversation ? resolveConversationAIToggleState(conversation) : { enabled: false, nextEnabled: true };
  const toggleBusy = Boolean(aiToggleBusy || badges?.status?.isAiModeSwitching);
  const toggleDisabled = !conversation || toggleBusy;
  const toggleClassName = aiToggleState.enabled
    ? "border-[#12b76a] bg-[#ecfdf3] text-[#067647] hover:bg-[#d1fadf]"
    : "border-[#f79009] bg-[#fffaeb] text-[#b54708] hover:bg-[#fef0c7]";
  return (
    <div className="border-b border-[#e5e9f2] bg-white px-4 py-3">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold text-[#172033]">{title}</div>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-[#697386]">
            <span className="truncate">{conversation?.conversation_id || " "}</span>
            {badges && <span className="bg-[#f2f4f7] px-1.5 py-0.5 text-[#475467]">{badges.replyLabel}</span>}
            {badges && <span className="bg-[#eef4ff] px-1.5 py-0.5 text-[#175cd3]" title={badges.modeTitle}>{badges.modeLabel}</span>}
            {badges?.runtimeLabel && <span className="bg-[#f0f9ff] px-1.5 py-0.5 text-[#026aa2]">{badges.runtimeLabel}</span>}
          </div>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          {callStatus && <span className="text-xs text-[#697386]">{callStatus}</span>}
          {conversation && (
            <>
              <button
                className={`h-8 border px-2 text-xs font-medium disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386] ${toggleClassName}`}
                type="button"
                disabled={toggleDisabled}
                onClick={() => onToggleAI?.(aiToggleState.nextEnabled)}
              >
                {toggleBusy ? "切换中" : (aiToggleState.enabled ? "AI托管" : "人工")}
              </button>
              <button
                className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                type="button"
                disabled={callDisabled}
                onClick={() => onStartCall?.("voice")}
              >
                {callBusy === "voice" ? "拨打中" : "语音"}
              </button>
              <button
                className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                type="button"
                disabled={callDisabled}
                onClick={() => onStartCall?.("video")}
              >
                {callBusy === "video" ? "拨打中" : "视频"}
              </button>
              <button
                className="h-8 border border-[#c0372b] bg-white px-2 text-xs font-medium text-[#b42318] hover:bg-[#fff4f2] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                type="button"
                disabled={callDisabled}
                onClick={() => onHangupCall?.()}
              >
                {callBusy === "hangup" ? "挂断中" : "挂断"}
              </button>
            </>
          )}
          <span className="text-xs text-[#697386]">{conversation?.account_name || ""}</span>
        </div>
      </div>
      {aiReplyErrorNotice?.visible && (
        <div className="mt-2 flex items-start gap-2 border border-[#fecdca] bg-[#fff4f2] px-2 py-2 text-xs text-[#b42318]">
          <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-[#f04438]" />
          <div className="min-w-0">
            <div className="font-semibold">{aiReplyErrorNotice.title}</div>
            <div className="mt-0.5 break-words text-[#b42318]">{aiReplyErrorNotice.error}</div>
          </div>
          <button
            className="ml-auto h-6 shrink-0 border border-[#fecdca] bg-white px-2 text-[11px] font-medium text-[#b42318] hover:bg-[#fee4e2]"
            type="button"
            onClick={() => onDismissAIReplyError?.(aiReplyErrorNotice.key)}
          >
            关闭
          </button>
        </div>
      )}
      {aiToggleError && <div className="mt-2 border border-[#fedf89] bg-[#fffaeb] px-2 py-1 text-xs text-[#b54708]">{aiToggleError}</div>}
      {error && <div className="mt-2 border border-[#f2b8b5] bg-[#fff4f2] px-2 py-1 text-xs text-[#b42318]">{error}</div>}
    </div>
  );
}

function callStatusText(callState, conversationID = "") {
  if (!callState || String(callState.conversationId || "").trim() !== String(conversationID || "").trim()) return "";
  if (callState.status === "hangup_queued") return "挂断已提交";
  if (callState.status === "queued") return callState.callType === "video" ? "视频已提交" : "语音已提交";
  return "";
}

function sidebarMixedMessageErrorMessage(error) {
  const messages = {
    device_required: "当前会话缺少可用设备",
    receiver_required: "当前会话缺少发送目标",
    messages_required: "请输入聚合消息",
  };
  return messages[error] || "聚合消息无法发送";
}

function MediaPreviewDialog({ preview, onClose }) {
  const [scale, setScale] = useState(1);
  const [imageRetry, setImageRetry] = useState(0);
  const [imageFailed, setImageFailed] = useState(false);

  useEffect(() => {
    setScale(1);
    setImageRetry(0);
    setImageFailed(false);
  }, [preview?.type, preview?.url]);

  useEffect(() => {
    if (!preview) return undefined;
    const handleKeyDown = (event) => {
      if (event.key === "Escape") onClose?.();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose, preview]);

  if (!preview) return null;

  const zoomOut = () => setScale((value) => Math.max(0.5, Math.round((value - 0.25) * 100) / 100));
  const zoomIn = () => setScale((value) => Math.min(3, Math.round((value + 0.25) * 100) / 100));
  const resetZoom = () => setScale(1);
  const retryImage = () => {
    setImageFailed(false);
    setImageRetry((value) => value + 1);
  };

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-black/80 p-4"
      role="dialog"
      aria-modal="true"
      aria-label={preview.title || "媒体预览"}
      onClick={() => onClose?.()}
    >
      <div className="grid max-h-full max-w-full gap-3" onClick={(event) => event.stopPropagation()}>
        <div className="flex items-center justify-between gap-3 text-sm text-white">
          <span className="min-w-0 truncate font-medium">{preview.title || "媒体预览"}</span>
          <div className="flex shrink-0 items-center gap-2">
            {preview.type === "image" && (
              <>
                <button className="h-8 border border-white/40 px-2 text-xs hover:bg-white/10" type="button" onClick={zoomOut}>缩小</button>
                <button className="h-8 border border-white/40 px-2 text-xs hover:bg-white/10" type="button" onClick={resetZoom}>{Math.round(scale * 100)}%</button>
                <button className="h-8 border border-white/40 px-2 text-xs hover:bg-white/10" type="button" onClick={zoomIn}>放大</button>
              </>
            )}
            <button className="h-8 border border-white/40 px-2 text-xs hover:bg-white/10" type="button" onClick={() => onClose?.()}>关闭</button>
          </div>
        </div>
        <div className="max-h-[82vh] max-w-[92vw] overflow-auto">
          {preview.type === "image" && imageFailed ? (
            <div className="grid min-h-56 min-w-80 place-items-center gap-3 border border-white/20 bg-black px-6 py-8 text-center text-sm text-white">
              <div>图片加载失败</div>
              <button className="h-8 border border-white/40 px-3 text-xs hover:bg-white/10" type="button" onClick={retryImage}>重试</button>
            </div>
          ) : preview.type === "image" ? (
            <img
              key={`${preview.url}#${imageRetry}`}
              className="max-h-[82vh] max-w-[92vw] object-contain"
              src={preview.url}
              alt={preview.title || "图片预览"}
              onError={() => setImageFailed(true)}
              style={{ transform: `scale(${scale})`, transformOrigin: "center center" }}
            />
          ) : (
            <video className="max-h-[82vh] max-w-[92vw] bg-black" src={preview.url} controls autoPlay />
          )}
        </div>
      </div>
    </div>
  );
}

function MessageRow({
  message,
  onResend,
  onRevoke,
  onRetryLocalMedia,
  onEditAISuggestion,
  onPrepareArchiveMedia,
  onRetryVoiceTranscription,
  onPreviewMedia,
  revokeNowMs,
}) {
  const outgoing = String(message.direction || "").toLowerCase() === "outgoing";
  const status = String(message.send_status || "").trim();
  const error = String(message.send_error || "").trim();
  const resendText = resolveResendStatusText(message);
  const revokeText = resolveRevokeStatusText(message);
  const canResend = canResendConversationMessage(message) && typeof onResend === "function";
  const canRevoke = canRevokeConversationMessage(message, revokeNowMs) && typeof onRevoke === "function";
  const canRetryLocalMedia = canRetryLocalMediaMessage(message) && typeof onRetryLocalMedia === "function";
  const canEditAISuggestion = canEditAISuggestionMessage(message) && typeof onEditAISuggestion === "function";
  const presentation = resolveWorkbenchMessagePresentation(message);
  return (
    <div className={`mb-3 flex ${outgoing ? "justify-end" : "justify-start"}`}>
      <div className={`max-w-[76%] border px-3 py-2 ${outgoing ? "border-[#bad5ff] bg-[#eaf3ff]" : "border-[#d8dde8] bg-white"}`}>
        <div className="mb-1 text-xs text-[#697386]">{message.display_name || message.sender_name || message.sender_id || "消息"}</div>
        <MessageContent
          message={message}
          presentation={presentation}
          onPrepareArchiveMedia={onPrepareArchiveMedia}
          onRetryVoiceTranscription={onRetryVoiceTranscription}
          onPreviewMedia={onPreviewMedia}
        />
        <div className="mt-2 flex items-center justify-end gap-2 text-xs text-[#8a94a6]">
          {status && <span>{status}</span>}
          {resendText && <span className={resendText === "补发失败" ? "text-[#b42318]" : "text-[#566072]"}>{resendText}</span>}
          {revokeText && <span className={revokeText === "撤回失败" ? "text-[#b42318]" : "text-[#566072]"}>{revokeText}</span>}
          {canResend && (
            <button
              className="border border-[#8a94a6] bg-white px-1.5 py-0.5 text-xs text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed]"
              type="button"
              onClick={() => onResend(message)}
            >
              补发
            </button>
          )}
          {canRevoke && (
            <button
              className="border border-[#8a94a6] bg-white px-1.5 py-0.5 text-xs text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed]"
              type="button"
              onClick={() => onRevoke(message)}
            >
              撤回
            </button>
          )}
          {canRetryLocalMedia && (
            <button
              className="border border-[#8a94a6] bg-white px-1.5 py-0.5 text-xs text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed]"
              type="button"
              onClick={() => onRetryLocalMedia(message)}
            >
              重试
            </button>
          )}
          {canEditAISuggestion && (
            <button
              className="border border-[#8a94a6] bg-white px-1.5 py-0.5 text-xs text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed]"
              type="button"
              onClick={() => onEditAISuggestion(message)}
            >
              编辑
            </button>
          )}
          <span>{formatTime(message.timestamp)}</span>
        </div>
        {error && <div className="mt-1 text-right text-xs text-[#b42318]">{error}</div>}
        {message.resend_error && <div className="mt-1 text-right text-xs text-[#b42318]">{message.resend_error}</div>}
        {message.revoke_error && <div className="mt-1 text-right text-xs text-[#b42318]">{message.revoke_error}</div>}
      </div>
    </div>
  );
}

function PendingArchiveMediaCard({ label, statusText, action, preparing, error, onPrepare }) {
  const canPrepare = Boolean(action && typeof onPrepare === "function");
  return (
    <div className="grid max-w-80 gap-2 border border-dashed border-[#cfd6e3] bg-[#f9fafc] px-3 py-3 text-sm text-[#566072]">
      <span>{label || action?.label || "媒体暂不可用"}</span>
      {canPrepare && (
        <button
          className="w-fit border border-[#b7cdf7] bg-white px-2 py-1 text-xs font-medium text-[#2f6fed] hover:bg-[#edf4ff] disabled:border-[#c4cad6] disabled:text-[#697386]"
          type="button"
          disabled={preparing}
          onClick={onPrepare}
        >
          {preparing ? action.loadingLabel : action.buttonLabel}
        </button>
      )}
      {error && <div className="text-xs text-[#b42318]">{error}</div>}
      {statusText && <div className="text-xs text-[#697386]">{statusText}</div>}
    </div>
  );
}

function VoiceTranscriptionNotice({ display, action, retrying, requestError, onRetry }) {
  const currentDisplay = display || (requestError ? { kind: "status", tone: "error", text: requestError } : null);
  if (!currentDisplay) return null;
  if (currentDisplay.kind === "transcript") {
    return (
      <div className="border border-[#d8dde8] bg-white/80 px-2 py-1 text-xs leading-5 text-[#566072]">
        {currentDisplay.text}
      </div>
    );
  }
  const tone = requestError ? "error" : currentDisplay.tone;
  const toneClass = tone === "error"
    ? "text-[#b42318]"
    : tone === "warning"
      ? "text-[#b54708]"
      : "text-[#697386]";
  const canRetry = Boolean(action && typeof onRetry === "function");
  return (
    <div className={`flex flex-wrap items-center gap-2 text-xs ${toneClass}`}>
      <span>{requestError || currentDisplay.text}</span>
      {canRetry && (
        <button
          className="border border-[#b7cdf7] bg-white px-2 py-0.5 text-xs font-medium text-[#2f6fed] hover:bg-[#edf4ff] disabled:border-[#c4cad6] disabled:text-[#697386]"
          type="button"
          title={action.label}
          disabled={retrying}
          onClick={onRetry}
        >
          {retrying ? action.loadingLabel : action.buttonLabel}
        </button>
      )}
    </div>
  );
}

function VoicePlaybackControl({ mediaUrl, durationText }) {
  const amrRecorderRef = useRef(null);
  const [playing, setPlaying] = useState(false);
  const [loading, setLoading] = useState(false);
  const [playError, setPlayError] = useState("");
  const mediaKind = resolveWorkbenchVoiceMediaKind(mediaUrl, "voice");

  useEffect(() => () => {
    const recorder = amrRecorderRef.current;
    if (recorder && typeof recorder.stop === "function") {
      try {
        recorder.stop();
      } catch {
        // ignore cleanup failures from the decoder.
      }
    }
    amrRecorderRef.current = null;
  }, [mediaUrl]);

  async function ensureAmrRecorder() {
    if (amrRecorderRef.current) return amrRecorderRef.current;
    const [{ default: BenzAMRRecorder }, response] = await Promise.all([
      import("benz-amr-recorder"),
      fetch(mediaUrl, { credentials: "same-origin" }),
    ]);
    if (!response.ok) {
      throw new Error(`AMR ${response.status}`);
    }
    const blob = await response.blob();
    const recorder = new BenzAMRRecorder();
    await recorder.initWithBlob(blob);
    recorder.onPlay(() => setPlaying(true));
    recorder.onPause(() => setPlaying(false));
    recorder.onStop(() => setPlaying(false));
    recorder.onEnded(() => setPlaying(false));
    amrRecorderRef.current = recorder;
    return recorder;
  }

  async function playWithAmrDecoder() {
    setLoading(true);
    try {
      const recorder = await ensureAmrRecorder();
      recorder.playOrResume();
    } catch {
      setPlaying(false);
      setPlayError("当前语音格式暂时无法播放。");
    } finally {
      setLoading(false);
    }
  }

  async function handleSpecialVoicePlayback() {
    setPlayError("");
    if (mediaKind === "silk") {
      setPlayError("当前语音为 SILK 格式，需服务端转码后播放。");
      return;
    }
    if (playing && amrRecorderRef.current) {
      try {
        amrRecorderRef.current.pauseOrResume();
      } catch {
        setPlayError("当前语音暂停失败，请重试。");
      }
      return;
    }
    await playWithAmrDecoder();
  }

  if (mediaKind !== "amr" && mediaKind !== "silk") {
    return (
      <div className="grid gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <audio
            className="h-8 max-w-full"
            src={mediaUrl}
            controls
            preload="metadata"
            onError={() => setPlayError("当前语音格式播放失败，请稍后重试。")}
          />
          {durationText && <span className="text-xs text-[#697386]">{durationText}</span>}
        </div>
        {playError && <div className="text-xs text-[#b42318]">{playError}</div>}
      </div>
    );
  }

  return (
    <div className="grid gap-1">
      <button
        className={`flex h-8 w-fit min-w-24 items-center gap-2 border px-2 text-xs font-medium ${
          playing
            ? "border-[#2f6fed] bg-[#edf4ff] text-[#1f5fc4]"
            : "border-[#cfd6e3] bg-white text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed]"
        }`}
        type="button"
        onClick={() => void handleSpecialVoicePlayback()}
      >
        <span>{loading ? "..." : playing ? "播放中" : durationText || "语音"}</span>
        <span className="text-[10px] uppercase text-[#697386]">{mediaKind}</span>
      </button>
      {playError && <div className="text-xs text-[#b42318]">{playError}</div>}
    </div>
  );
}

function MessageContent({ message, presentation, onPrepareArchiveMedia, onRetryVoiceTranscription, onPreviewMedia }) {
  const [imageRetry, setImageRetry] = useState(0);
  const [imageFailed, setImageFailed] = useState(false);
  const [prepareLoading, setPrepareLoading] = useState(false);
  const [prepareError, setPrepareError] = useState("");
  const [voiceRetryStatus, setVoiceRetryStatus] = useState("");
  const [voiceRetryText, setVoiceRetryText] = useState("");
  const [voiceRetryTranscriptionError, setVoiceRetryTranscriptionError] = useState("");
  const [voiceRetrying, setVoiceRetrying] = useState(false);
  const [voiceRetryRequestError, setVoiceRetryRequestError] = useState("");
  const prepareAction = buildWorkbenchArchiveMediaPrepareAction(message, presentation);
  const voiceRetryAction = buildWorkbenchVoiceTranscriptionRetryAction(message, { localStatus: voiceRetryStatus });
  const voiceTranscriptionDisplay = resolveWorkbenchVoiceTranscriptionDisplay(message, {
    status: voiceRetryStatus,
    voiceText: voiceRetryText,
    error: voiceRetryTranscriptionError,
  });

  useEffect(() => {
    setImageRetry(0);
    setImageFailed(false);
  }, [presentation.kind, presentation.mediaUrl]);

  useEffect(() => {
    setPrepareLoading(false);
    setPrepareError("");
  }, [prepareAction?.taskId]);

  useEffect(() => {
    setVoiceRetryStatus("");
    setVoiceRetryText("");
    setVoiceRetryTranscriptionError("");
    setVoiceRetrying(false);
    setVoiceRetryRequestError("");
  }, [message?.archive_msgid, message?.voice_text, message?.voice_transcription_status]);

  const handlePrepareMedia = async () => {
    if (!prepareAction || prepareLoading || typeof onPrepareArchiveMedia !== "function") return;
    setPrepareLoading(true);
    setPrepareError("");
    try {
      await onPrepareArchiveMedia(prepareAction.taskId);
      setPrepareLoading(false);
    } catch (err) {
      setPrepareLoading(false);
      setPrepareError(err.message || String(err) || "媒体加载失败，请稍后重试");
    }
  };

  const renderPrepareButton = () => (
    prepareAction ? (
      <div className="mt-2 grid gap-1">
        <button
          className="w-fit border border-[#b7cdf7] bg-white px-2 py-1 text-xs font-medium text-[#2f6fed] hover:bg-[#edf4ff] disabled:border-[#c4cad6] disabled:text-[#697386]"
          type="button"
          disabled={prepareLoading}
          onClick={handlePrepareMedia}
        >
          {prepareLoading ? prepareAction.loadingLabel : prepareAction.buttonLabel}
        </button>
        {prepareError && <div className="text-xs text-[#b42318]">{prepareError}</div>}
        {presentation.statusText && <div className="text-xs text-[#697386]">{presentation.statusText}</div>}
      </div>
    ) : null
  );

  const handleRetryVoiceTranscription = async (event) => {
    event?.preventDefault?.();
    event?.stopPropagation?.();
    if (!voiceRetryAction || voiceRetrying || typeof onRetryVoiceTranscription !== "function") return;
    if (!voiceRetryAction.body.archive_msgid) {
      setVoiceRetryRequestError(voiceRetryAction.missingTaskMessage);
      return;
    }
    setVoiceRetrying(true);
    setVoiceRetryRequestError("");
    try {
      const response = await onRetryVoiceTranscription(voiceRetryAction.body);
      const result = normalizeWorkbenchVoiceTranscriptionRetryResult(response);
      setVoiceRetryStatus(result.status);
      setVoiceRetryText(result.voiceText);
      setVoiceRetryTranscriptionError(result.error);
    } catch (err) {
      setVoiceRetryRequestError(formatWorkbenchVoiceTranscriptionRetryError(err));
    } finally {
      setVoiceRetrying(false);
    }
  };

  if (presentation.kind === "image" && presentation.mediaUrl) {
    const preview = buildWorkbenchMediaPreview(presentation);
    if (imageFailed) {
      return (
        <div className="grid max-w-80 gap-2 border border-[#fecdca] bg-[#fff4f2] px-3 py-3 text-sm text-[#b42318]">
          <span>图片加载失败</span>
          <button
            className="w-fit border border-[#fecdca] bg-white px-2 py-1 text-xs font-medium text-[#b42318] hover:bg-[#fee4e2]"
            type="button"
            onClick={() => {
              setImageFailed(false);
              setImageRetry((value) => value + 1);
            }}
          >
            重试
          </button>
        </div>
      );
    }
    return (
      <button className="block max-w-full text-left" type="button" onClick={() => preview && onPreviewMedia?.(preview)}>
        <img
          key={`${presentation.mediaUrl}#${imageRetry}`}
          className="max-h-64 max-w-full border border-[#d8dde8] object-contain"
          src={presentation.mediaUrl}
          alt={presentation.fileName || "图片"}
          loading="lazy"
          onError={() => setImageFailed(true)}
        />
      </button>
    );
  }
  if (presentation.kind === "image") {
    return (
      <PendingArchiveMediaCard
        label={prepareAction?.label || presentation.text || "图片暂不可预览"}
        statusText={presentation.statusText}
        action={prepareAction}
        preparing={prepareLoading}
        error={prepareError}
        onPrepare={handlePrepareMedia}
      />
    );
  }
  if (presentation.kind === "video" && presentation.mediaUrl) {
    const preview = buildWorkbenchMediaPreview(presentation);
    return (
      <div className="grid gap-2">
        <video
          className="aspect-video w-80 max-w-full bg-black"
          src={presentation.mediaUrl}
          controls
          preload="metadata"
        />
        {preview && (
          <button
            className="w-fit border border-[#cfd6e3] bg-white px-2 py-1 text-xs font-medium text-[#172033] hover:border-[#2f6fed] hover:text-[#2f6fed]"
            type="button"
            onClick={() => onPreviewMedia?.(preview)}
          >
            预览
          </button>
        )}
      </div>
    );
  }
  if (presentation.kind === "video") {
    return (
      <PendingArchiveMediaCard
        label={prepareAction?.label || presentation.text || "视频暂不可预览"}
        statusText={presentation.statusText}
        action={prepareAction}
        preparing={prepareLoading}
        error={prepareError}
        onPrepare={handlePrepareMedia}
      />
    );
  }
  if (presentation.kind === "voice") {
    if (!presentation.mediaUrl && prepareAction) {
      return (
        <PendingArchiveMediaCard
          label={prepareAction.label}
          statusText={presentation.statusText}
          action={prepareAction}
          preparing={prepareLoading}
          error={prepareError}
          onPrepare={handlePrepareMedia}
        />
      );
    }
    return (
      <div className="grid gap-2 text-sm leading-6 text-[#172033]">
        <div className="flex flex-wrap items-center gap-2">
          {presentation.mediaUrl ? (
            <VoicePlaybackControl mediaUrl={presentation.mediaUrl} durationText={presentation.voiceDurationText} />
          ) : (
            <span>{presentation.text || "[语音消息]"}</span>
          )}
          {!presentation.mediaUrl && presentation.voiceDurationText && <span className="text-xs text-[#697386]">{presentation.voiceDurationText}</span>}
        </div>
        <VoiceTranscriptionNotice
          display={voiceTranscriptionDisplay}
          action={voiceRetryAction}
          retrying={voiceRetrying}
          requestError={voiceRetryRequestError}
          onRetry={handleRetryVoiceTranscription}
        />
      </div>
    );
  }
  if (presentation.kind === "file") {
    const body = (
      <div className="flex max-w-80 items-center gap-2 border border-[#d8dde8] bg-white px-2 py-2 text-sm text-[#172033]">
        <span className="grid h-8 w-10 shrink-0 place-items-center border border-[#cfd6e3] bg-[#f9fafc] text-[10px] font-semibold text-[#566072]">
          {presentation.extension || "FILE"}
        </span>
        <span className="min-w-0 truncate">{presentation.fileName || presentation.text || "file"}</span>
      </div>
    );
    if (presentation.mediaUrl) {
      return (
        <a className="inline-block max-w-full hover:underline" href={presentation.mediaUrl} target="_blank" rel="noreferrer">
          {body}
        </a>
      );
    }
    return (
      <>
        {body}
        {renderPrepareButton()}
      </>
    );
  }
  return (
    <div className="whitespace-pre-wrap break-words text-sm leading-6 text-[#172033]">
      {presentation.text || presentation.statusText || presentation.kind}
    </div>
  );
}

function resolveResendStatusText(message = {}) {
  const status = String(message?.resend_status || "").trim().toLowerCase();
  if (status === "pending") return "补发中";
  if (status === "queued" || status === "running") return "补发已提交";
  if (status === "success") return "补发成功";
  if (status === "failed") return "补发失败";
  return "";
}

function resolveRevokeStatusText(message = {}) {
  const status = String(message?.revoke_status || "").trim().toLowerCase();
  if (status === "pending" || status === "queued" || status === "running") return "撤回中";
  if (status === "success") return "已撤回";
  if (status === "failed") return "撤回失败";
  return "";
}

function ConversationSkeleton() {
  return Array.from({ length: 8 }).map((_, index) => (
    <div key={index} className="border-b border-[#edf0f5] px-3 py-3">
      <div className="h-4 w-2/3 animate-pulse bg-[#e1e6ef]" />
      <div className="mt-2 h-3 w-full animate-pulse bg-[#edf0f5]" />
    </div>
  ));
}

function MessageSkeleton() {
  return Array.from({ length: 6 }).map((_, index) => (
    <div key={index} className="mb-3 h-16 w-2/3 animate-pulse border border-[#d8dde8] bg-white" />
  ));
}

function EmptyState({ label }) {
  return <div className="p-6 text-center text-sm text-[#697386]">{label}</div>;
}

function formatTime(value) {
  const text = String(value || "").trim();
  if (!text) return "";
  return text.replace("T", " ").replace("+08:00", "").slice(0, 16);
}
