import { useState } from "react";
import { api, useMock } from "../api.js";
import PostCard from "../components/PostCard.jsx";

export default function SearchPage({ session, navigate }) {
  const [keyword, setKeyword] = useState("");
  const [searched, setSearched] = useState("");
  const [items, setItems] = useState([]);
  const [state, setState] = useState("idle");
  const [error, setError] = useState("");

  async function search(event) {
    event.preventDefault();
    const value = keyword.trim();
    if (!value) return;
    setState("loading");
    setError("");
    setSearched(value);
    try {
      const data = await api.searchPosts(value, session.token);
      setItems(data.items || []);
      setState("ready");
    } catch (cause) {
      setItems([]);
      setError(cause.message);
      setState("error");
    }
  }

  return <div className="search-page">
    <header className="page-heading"><div><span>{useMock ? "MOCK SEARCH" : "POST SEARCH"}</span><h1>搜索帖子</h1></div></header>
    <form className="search-form" onSubmit={search} role="search">
      <span aria-hidden="true">⌕</span>
      <input value={keyword} onChange={(event) => setKeyword(event.target.value)} maxLength={100} placeholder="搜索帖子正文" aria-label="搜索帖子正文" autoFocus />
      <button className="primary-button" disabled={!keyword.trim() || state === "loading"}>{state === "loading" ? "搜索中" : "搜索"}</button>
    </form>
    <section className="feed-list" aria-live="polite">
      {state === "idle" && <SearchState title="查找你感兴趣的内容" detail="输入帖子正文中的关键词开始搜索。" />}
      {state === "loading" && <SearchLoading />}
      {state === "error" && <SearchState title="暂时无法搜索" detail={error} />}
      {state === "ready" && items.length === 0 && <SearchState title={`没有找到“${searched}”`} detail="换一个关键词再试试。" />}
      {state === "ready" && items.map((post) => <PostCard key={post.post_id || post.id} initialPost={post} token={session.token} navigate={navigate} />)}
    </section>
  </div>;
}

function SearchState({ title, detail }) {
  return <div className="empty-state"><span>⌕</span><h2>{title}</h2><p>{detail}</p></div>;
}

function SearchLoading() {
  return <div className="loading-list">{[1, 2, 3].map((item) => <div className="skeleton" key={item}><i /><div><b /><span /><span /></div></div>)}</div>;
}
