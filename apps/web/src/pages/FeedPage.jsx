import { useEffect, useState } from "react";
import { api, useMock } from "../api.js";
import PostCard from "../components/PostCard.jsx";
import PostComposer from "../components/PostComposer.jsx";

export default function FeedPage({ scene, session, navigate, compose = false }) {
  const [items, setItems] = useState([]);
  const [state, setState] = useState("loading");
  const [error, setError] = useState("");
  async function load() { setState("loading"); setError(""); try { const data = scene === "hot" ? await api.hotFeed(session.token) : await api.followingFeed(session.token); setItems(data.items || []); setState("ready"); } catch (cause) { setError(cause.message); setState("error"); } }
  useEffect(() => { if (scene === "following" && !session.token) { navigate("/auth"); return; } load(); }, [scene, session.token]);
  return <div className="feed-page"><header className="page-heading"><div><span>{useMock ? "MOCK DATA" : "LIVE FEED"}</span><h1>{compose ? "发布帖子" : scene === "hot" ? "此刻热门" : "关注动态"}</h1></div>{!compose && <button className="refresh-button" onClick={load}>刷新</button>}</header>{(compose || session.user) && <PostComposer expanded={compose} token={session.token} user={session.user} onCreated={(post) => { setItems((current) => [{ ...post, author_nickname: session.user.nickname, author_avatar_url: session.user.avatar_url }, ...current]); if (compose) navigate("/following"); }} />}{!compose && <section className="feed-list">{state === "loading" && <Loading />}{state === "error" && <Empty title="暂时无法读取帖子" detail={error} action={load} />}{state === "ready" && items.length === 0 && <Empty title="这里还很安静" detail="关注一些人，或先发布第一条帖子。" />}{items.map((post) => <PostCard key={post.post_id || post.id} initialPost={post} token={session.token} navigate={navigate} />)}</section>}</div>;
}

function Loading() { return <div className="loading-list">{[1, 2, 3].map((item) => <div className="skeleton" key={item}><i /><div><b /><span /><span /></div></div>)}</div>; }
function Empty({ title, detail, action }) { return <div className="empty-state"><span>◌</span><h2>{title}</h2><p>{detail}</p>{action && <button className="text-button" onClick={action}>重新加载</button>}</div>; }
