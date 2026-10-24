import { useEffect, useState } from "react";
import { api, assetURL } from "../api.js";

export default function MessagesPage({ token, onRead, navigate }) {
  const [items, setItems] = useState([]);
  const [error, setError] = useState("");
  useEffect(() => { if (!token) { navigate("/auth"); return; } api.messages(token).then(async (data) => { const next = data.items || []; setItems(next); const unread = next.filter((item) => !item.is_read).map((item) => item.id); if (unread.length) { await api.markRead(unread, token); setItems((current) => current.map((item) => ({ ...item, is_read: true }))); onRead?.(); } }).catch((cause) => setError(cause.message)); }, [token]);
  return <div className="messages-page"><header className="page-heading"><div><span>INBOX</span><h1>消息</h1></div></header>{error && <p className="form-error page-error">{error}</p>}<section className="message-list">{items.map((item) => <article key={item.id}><i className={!item.is_read ? "unread" : ""} /><img src={assetURL(item.actor_avatar_url) || `https://api.dicebear.com/9.x/initials/svg?seed=${item.actor_nickname || "notice"}`} alt="" /><div><b>{item.title}</b>{item.content && <p>{item.content}</p>}<time>{new Date(item.created_at).toLocaleString("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}</time></div></article>)}{items.length === 0 && <div className="empty-state"><span>◇</span><h2>暂无消息</h2><p>新的互动会出现在这里。</p></div>}</section></div>;
}
