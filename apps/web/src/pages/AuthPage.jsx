import { useState } from "react";
import { api, useMock } from "../api.js";

export default function AuthPage({ onAuth, navigate }) {
  const [register, setRegister] = useState(false);
  const [form, setForm] = useState({ account: useMock ? "mori" : "", password: useMock ? "demo1234" : "", nickname: "" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event) {
    event.preventDefault(); setBusy(true); setError("");
    try { if (register) await api.register(form); const result = await api.login(form); const profile = await api.me(result.access_token); onAuth(result.access_token, profile); navigate("/following"); } catch (cause) { setError(cause.message); } finally { setBusy(false); }
  }
  return <main className="auth-page"><button className="wordmark auth-brand" onClick={() => navigate("/hot")}>morrow<span>•</span></button><section className="auth-copy"><p>POSTS, PEOPLE, MOMENTS</p><h1>留住正在<br />发生的事。</h1><span>关注你在意的人，也让文字和图片自然抵达。</span></section><section className="auth-form-wrap"><form onSubmit={submit}><header><h2>{register ? "创建账号" : "欢迎回来"}</h2><p>{useMock ? "Mock 模式已开启，直接进入即可。" : "使用你的账号继续。"}</p></header>{register && <label>昵称<input required maxLength={40} value={form.nickname} onChange={(e) => setForm({ ...form, nickname: e.target.value })} /></label>}<label>账号<input required value={form.account} onChange={(e) => setForm({ ...form, account: e.target.value })} /></label><label>密码<input required minLength={8} type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} /></label>{error && <p className="form-error">{error}</p>}<button className="primary-button wide" disabled={busy}>{busy ? "请稍候…" : register ? "注册并登录" : "登录"}</button><button type="button" className="switch-button" onClick={() => { setRegister(!register); setError(""); }}>{register ? "已有账号？登录" : "没有账号？注册"}</button></form></section></main>;
}
