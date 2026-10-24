import { useEffect, useState } from "react";
import { api, assetURL } from "../api.js";
import PostCard from "../components/PostCard.jsx";

export default function ProfilePage({ userId, session, navigate }) {
  const own = !userId || Number(userId) === Number(session.user?.id);
  const [profile, setProfile] = useState(own ? session.user : null);
  const [posts, setPosts] = useState([]);
  const [following, setFollowing] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { (async () => { try { const nextProfile = own ? await api.me(session.token) : await api.profile(userId); const data = own ? await api.myPosts(session.token) : await api.userPosts(userId); setProfile(nextProfile); setPosts((data.items || []).map((post) => ({ ...post, post_id: post.post_id || post.id, author_nickname: nextProfile.nickname, author_avatar_url: nextProfile.avatar_url }))); } catch (cause) { setError(cause.message); } })(); }, [userId, own, session.token]);
  async function toggleFollow() { if (!session.token) return navigate("/auth"); const next = !following; try { await api.follow(Number(userId), next, session.token); setFollowing(next); setProfile((current) => ({ ...current, follower_count: Math.max(0, current.follower_count + (next ? 1 : -1)) })); } catch (cause) { setError(cause.message); } }
  if (!profile) return <div className="empty-state"><p>{error || "正在读取主页…"}</p></div>;
  return <div className="profile-page"><section className="profile-header"><div><span>PROFILE</span><h1>{profile.nickname}</h1><p>{profile.bio || "这个人还没有留下简介。"}</p><div className="profile-stats"><b>{profile.follower_count || 0}<small>粉丝</small></b><b>{profile.following_count || 0}<small>关注</small></b><b>{profile.work_count || posts.length}<small>帖子</small></b></div>{!own && <button className={following ? "secondary-button" : "primary-button"} onClick={toggleFollow}>{following ? "已关注" : "关注"}</button>}</div><img src={assetURL(profile.avatar_url) || `https://api.dicebear.com/9.x/initials/svg?seed=${profile.nickname}`} alt="" /></section>{error && <p className="form-error page-error">{error}</p>}<div className="section-label"><span>POSTS</span><b>{posts.length}</b></div><section className="feed-list">{posts.map((post) => <PostCard key={post.post_id} initialPost={post} token={session.token} navigate={navigate} />)}{posts.length === 0 && <div className="empty-state"><p>还没有发布过帖子。</p></div>}</section></div>;
}
