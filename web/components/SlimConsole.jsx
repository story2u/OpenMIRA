"use client";

import { useEffect, useState } from "react";
import { normalizeMessages, normalizeSOPCollection, requestJSON } from "../lib/slimApi.js";

export function SlimConsole() {
  const [conversationId, setConversationId] = useState("demo");
  const [senderName, setSenderName] = useState("customer");
  const [messageText, setMessageText] = useState("你好");
  const [replyText, setReplyText] = useState("收到，我们马上处理。");
  const [messages, setMessages] = useState([]);
  const [flows, setFlows] = useState([]);
  const [policies, setPolicies] = useState([]);
  const [flowID, setFlowID] = useState("default");
  const [policyID, setPolicyID] = useState("welcome");
  const [policyText, setPolicyText] = useState("欢迎咨询，我先了解一下你的需求。");
  const [notice, setNotice] = useState("");

  async function refreshMessages() {
    const payload = await requestJSON(`conversations/${encodeURIComponent(conversationId)}/messages`);
    setMessages(normalizeMessages(payload));
  }

  async function refreshSOP() {
    const [flowPayload, policyPayload] = await Promise.all([
      requestJSON("admin/sop/flows"),
      requestJSON("admin/sop/policies"),
    ]);
    setFlows(normalizeSOPCollection(flowPayload, "flows"));
    setPolicies(normalizeSOPCollection(policyPayload, "policies"));
  }

  useEffect(() => {
    void refreshSOP();
  }, []);

  async function run(label, action) {
    setNotice("");
    try {
      await action();
      setNotice(label);
    } catch (err) {
      setNotice(err.message || String(err));
    }
  }

  return (
    <main className="page">
      <div className="shell">
        <header className="topbar">
          <div>
            <h1>IM Slim</h1>
            <p>只保留消息收发和 SOP 的 Go + Next.js 工作面。</p>
          </div>
          {notice ? <p className={notice.includes("失败") ? "notice" : "notice ok"}>{notice}</p> : null}
        </header>

        <section className="grid">
          <div className="panel">
            <h2>消息</h2>
            <div className="row">
              <label>
                会话 ID
                <input value={conversationId} onChange={(event) => setConversationId(event.target.value)} />
              </label>
              <label>
                发送方
                <input value={senderName} onChange={(event) => setSenderName(event.target.value)} />
              </label>
            </div>
            <label>
              入站内容
              <textarea value={messageText} onChange={(event) => setMessageText(event.target.value)} />
            </label>
            <label>
              回复内容
              <textarea value={replyText} onChange={(event) => setReplyText(event.target.value)} />
            </label>
            <div className="actions">
              <button
                type="button"
                onClick={() => run("入站消息已写入", async () => {
                  await requestJSON("messages/incoming", {
                    method: "POST",
                    body: { conversation_id: conversationId, sender_name: senderName, content: messageText },
                  });
                  await refreshMessages();
                })}
              >
                写入入站
              </button>
              <button
                type="button"
                onClick={() => run("文本消息已发送", async () => {
                  await requestJSON("send/text", {
                    method: "POST",
                    body: { conversation_id: conversationId, sender_name: "agent", content: replyText },
                  });
                  await refreshMessages();
                })}
              >
                发送文本
              </button>
              <button className="secondary" type="button" onClick={() => run("消息已刷新", refreshMessages)}>
                刷新
              </button>
            </div>
            <div className="messages">
              {messages.length === 0 ? <p className="empty">暂无消息</p> : messages.map((message) => (
                <article className="message" key={message.id}>
                  <header>
                    <span>{message.direction} / {message.senderName}</span>
                    <span>{message.timestamp}</span>
                  </header>
                  <p>{message.content}</p>
                </article>
              ))}
            </div>
          </div>

          <div className="panel">
            <h2>SOP</h2>
            <div className="row">
              <label>
                Flow ID
                <input value={flowID} onChange={(event) => setFlowID(event.target.value)} />
              </label>
              <label>
                Policy ID
                <input value={policyID} onChange={(event) => setPolicyID(event.target.value)} />
              </label>
            </div>
            <label>
              SOP 回复
              <textarea value={policyText} onChange={(event) => setPolicyText(event.target.value)} />
            </label>
            <div className="actions">
              <button
                type="button"
                onClick={() => run("SOP flow 已保存", async () => {
                  await requestJSON("admin/sop/flows", {
                    method: "POST",
                    body: { flow_id: flowID, flow_name: flowID, enabled: true },
                  });
                  await refreshSOP();
                })}
              >
                保存 Flow
              </button>
              <button
                type="button"
                onClick={() => run("SOP policy 已保存", async () => {
                  await requestJSON("admin/sop/policies", {
                    method: "POST",
                    body: { policy_id: policyID, flow_id: flowID, name: policyID, reply_text: policyText, enabled: true },
                  });
                  await refreshSOP();
                })}
              >
                保存 Policy
              </button>
              <button
                className="secondary"
                type="button"
                onClick={() => run("SOP 任务已创建", async () => {
                  await requestJSON("admin/sop/dispatch-tasks", {
                    method: "POST",
                    body: { conversation_id: conversationId, flow_id: flowID, policy_id: policyID },
                  });
                })}
              >
                创建任务
              </button>
            </div>
            <div className="list">
              {flows.map((flow) => (
                <div className="item" key={flow.flow_id}>
                  <strong>{flow.flow_id}</strong>
                  <span>{flow.flow_name} / {flow.enabled ? "enabled" : "disabled"}</span>
                </div>
              ))}
              {policies.map((policy) => (
                <div className="item" key={policy.policy_id}>
                  <strong>{policy.policy_id}</strong>
                  <span>{policy.flow_id} / {policy.reply_text || policy.name}</span>
                </div>
              ))}
            </div>
          </div>
        </section>
      </div>
    </main>
  );
}
