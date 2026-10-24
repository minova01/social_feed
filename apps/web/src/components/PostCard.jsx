import { useState } from "react";
import { api, assetURL } from "../api.js";

const relativeTime = (value) => { const delta = Math.max(0, Date.now() - Number(value)); if (delta < 60_000) return "刚刚"; if (delta < 3_600_000) return `${Math.floor(delta / 60_000)} 分钟`; if (delta < 86_400_000) return `${Math.floor(delta / 3_600_000)} 小时`; return `${Math.floor(delta / 86_400_000)} 天`; };

export default function PostCard({ initialPost, token, navigate }) {
  const [post, setPost] = useState(initialPost);
  const [commentsOpen, setCommentsOpen] = useState(false);
  const [comments, setComments] = useState([]);
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const id = post.post_id || post.id;
  const author = post.author_nickname || post.nickname || `用户 ${post.author_id}`;

  async function toggle(type) {
    if (!token) return navigate("/auth");
    const flag = type === "like" ? "liked" : "favorited";
    const count = type === "like" ? "like_count" : "favorite_count";
    const active = !post[flag];
    setPost((current) => ({ ...current, [flag]: active, [count]: Math.max(0, Number(current[count] || 0) + (active ? 1 : -1)) }));
    try { const result = await api.setAction(id, type, active, token); setPost((current) => ({ ...current, [flag]: result.active, like_count: result.like_count, favorite_count: result.favorite_count })); } catch (cause) { setPost((current) => ({ ...current, [flag]: !active, [count]: Math.max(0, Number(current[count] || 0) + (active ? -1 : 1)) })); setError(cause.message); }
  }
  async function openComments() { setCommentsOpen((open) => !open); if (!commentsOpen && comments.length === 0) { try { setComments((await api.listComments(id)).items || []); } catch (cause) { setError(cause.message); } } }
  async function submitComment(event) { event.preventDefault(); if (!token) return navigate("/auth"); if (!comment.trim()) return; try { const created = await api.createComment(id, comment.trim(), token); setComments((items) => [...items, created]); setPost((current) => ({ ...current, comment_count: created.comment_count })); setComment(""); } catch (cause) { setError(cause.message); } }

  return <article className="post-card">
    <button className="avatar-link" onClick={() => navigate(`/users/${post.author_id}`)}><img className="avatar" src={assetURL(post.author_avatar_url) || `https://api.dicebear.com/9.x/initials/svg?seed=${author}`} alt="" /></button>
    <div className="post-content"><header><button onClick={() => navigate(`/users/${post.author_id}`)}>{author}</button><span>· {relativeTime(post.published_at)}</span></header><p>{post.content}</p>
      {post.image_urls?.length > 0 && <div className={`image-grid count-${post.image_urls.length}`}>{post.image_urls.map((url, index) => <img key={`${url}-${index}`} src={assetURL(url)} alt="帖子图片" loading="lazy" />)}</div>}
      <div className="interaction-bar"><button className={post.liked ? "active" : ""} onClick={() => toggle("like")}><span>{post.liked ? "♥" : "♡"}</span>{post.like_count || 0}</button><button onClick={openComments}><span>○</span>{post.comment_count || 0}</button><button className={post.favorited ? "active" : ""} onClick={() => toggle("favorite")}><span>{post.favorited ? "◆" : "◇"}</span>{post.favorite_count || 0}</button></div>
      {error && <p className="inline-error">{error}</p>}
      {commentsOpen && <section className="comments"><form onSubmit={submitComment}><input value={comment} maxLength={1000} placeholder="写下回复…" onChange={(event) => setComment(event.target.value)} /><button>回复</button></form>{comments.map((item) => <div className="comment" key={item.id}><img src={assetURL(item.user_avatar_url) || `https://api.dicebear.com/9.x/initials/svg?seed=${item.user_nickname}`} alt="" /><p><b>{item.user_nickname}</b>{item.content}</p></div>)}{comments.length === 0 && <small>还没有回复</small>}</section>}
    </div>
  </article>;
}
