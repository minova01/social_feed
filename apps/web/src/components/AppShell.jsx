const nav = [{ path: "/following", label: "关注", icon: "⌂" }, { path: "/hot", label: "热门", icon: "⌁" }, { path: "/search", label: "搜索", icon: "⌕" }, { path: "/compose", label: "发布", icon: "+" }, { path: "/messages", label: "消息", icon: "◇" }, { path: "/me", label: "我的", icon: "○" }];

export default function AppShell({ route, user, unread, children, navigate, logout }) {
  return <div className="app-frame">
    <header className="topbar"><button className="wordmark" onClick={() => navigate("/following")}>morrow<span>•</span></button><div className="top-actions"><span className="mode-label">POST FEED</span>{user ? <button className="avatar-button" onClick={() => navigate("/me")}><img src={user.avatar_url || `https://api.dicebear.com/9.x/initials/svg?seed=${user.nickname}`} alt="" /></button> : <button className="text-button" onClick={() => navigate("/auth")}>登录</button>}</div></header>
    <aside className="side-nav" aria-label="主导航">{nav.map((item) => <button key={item.path} className={route === item.path ? "nav-item active" : "nav-item"} onClick={() => navigate(item.path)}><span>{item.icon}</span><b>{item.label}</b>{item.path === "/messages" && unread > 0 && <i>{unread > 99 ? "99+" : unread}</i>}</button>)}{user && <button className="nav-item logout" onClick={logout}><span>↗</span><b>退出</b></button>}</aside>
    <main className="main-column">{children}</main>
    <nav className="mobile-nav">{nav.map((item) => <button key={item.path} className={route === item.path ? "active" : ""} onClick={() => navigate(item.path)}><span>{item.icon}</span>{item.path === "/messages" && unread > 0 && <i />}</button>)}</nav>
  </div>;
}
