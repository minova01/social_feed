import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "./api.js";
import AppShell from "./components/AppShell.jsx";
import AuthPage from "./pages/AuthPage.jsx";
import FeedPage from "./pages/FeedPage.jsx";
import MessagesPage from "./pages/MessagesPage.jsx";
import ProfilePage from "./pages/ProfilePage.jsx";
import SearchPage from "./pages/SearchPage.jsx";

const TOKEN_KEY = "social.accessToken";
const USER_KEY = "social.user";
const readUser = () => { try { return JSON.parse(localStorage.getItem(USER_KEY) || "null"); } catch { return null; } };
const normalizeRoute = (path) => ["/", "/timeline", "/recommend", "/hotfeed", "/upload", "/profile"].includes(path) ? ({ "/": "/following", "/timeline": "/hot", "/recommend": "/hot", "/hotfeed": "/hot", "/upload": "/compose", "/profile": "/me" }[path]) : path;

export default function App() {
  const [route, setRoute] = useState(() => normalizeRoute(window.location.pathname));
  const [token, setToken] = useState(() => localStorage.getItem(TOKEN_KEY) || "");
  const [user, setUser] = useState(readUser);
  const [unread, setUnread] = useState(0);
  const navigate = useCallback((path) => { window.history.pushState({}, "", path); setRoute(normalizeRoute(path)); window.scrollTo({ top: 0, behavior: "smooth" }); }, []);
  useEffect(() => { const pop = () => setRoute(normalizeRoute(window.location.pathname)); window.addEventListener("popstate", pop); return () => window.removeEventListener("popstate", pop); }, []);
  const refreshUnread = useCallback(() => { if (!token) return setUnread(0); api.unread(token).then((data) => setUnread(Number(data.unread_count || 0))).catch(() => setUnread(0)); }, [token]);
  useEffect(refreshUnread, [refreshUnread, route]);
  const session = useMemo(() => ({ token, user }), [token, user]);

  function setAuth(nextToken, nextUser) { setToken(nextToken); setUser(nextUser); localStorage.setItem(TOKEN_KEY, nextToken); localStorage.setItem(USER_KEY, JSON.stringify(nextUser)); }
  async function logout() { try { await api.logout(token); } finally { setToken(""); setUser(null); localStorage.removeItem(TOKEN_KEY); localStorage.removeItem(USER_KEY); navigate("/hot"); } }
  if (route === "/auth" || route === "/login") return <AuthPage onAuth={setAuth} navigate={navigate} />;

  let page;
  const publicMatch = route.match(/^\/users\/(\d+)$/);
  if (publicMatch) page = <ProfilePage userId={publicMatch[1]} session={session} navigate={navigate} />;
  else if (route === "/me") page = token ? <ProfilePage session={session} navigate={navigate} /> : <AuthPage onAuth={setAuth} navigate={navigate} />;
  else if (route === "/messages") page = <MessagesPage token={token} onRead={refreshUnread} navigate={navigate} />;
  else if (route === "/search") page = <SearchPage session={session} navigate={navigate} />;
  else if (route === "/compose") page = token ? <FeedPage compose scene="following" session={session} navigate={navigate} /> : <AuthPage onAuth={setAuth} navigate={navigate} />;
  else page = <FeedPage scene={route === "/following" ? "following" : "hot"} session={session} navigate={navigate} />;

  return <AppShell route={route} user={user} unread={unread} navigate={navigate} logout={logout}>{page}</AppShell>;
}
