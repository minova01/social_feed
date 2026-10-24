import { useRef, useState } from "react";
import { api, assetURL } from "../api.js";

export default function PostComposer({ token, user, onCreated, expanded = false }) {
  const [content, setContent] = useState("");
  const [files, setFiles] = useState([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const inputRef = useRef(null);
  const previews = files.map((file) => ({ file, url: URL.createObjectURL(file) }));

  function selectFiles(event) {
    setFiles((current) => [...current, ...Array.from(event.target.files || [])].slice(0, 9));
    event.target.value = "";
  }
  async function submit() {
    if (!content.trim() || busy) return;
    setBusy(true); setError("");
    try {
      const uploaded = await Promise.all(files.map((file) => api.upload(file, token)));
      const post = await api.createPost({ content: content.trim(), image_urls: uploaded.map((item) => item.url) }, token);
      setContent(""); setFiles([]); onCreated?.(post);
    } catch (cause) { setError(cause.message); } finally { setBusy(false); }
  }
  return <section className={expanded ? "composer expanded" : "composer"}>
    <img className="avatar" src={assetURL(user?.avatar_url) || `https://api.dicebear.com/9.x/initials/svg?seed=${user?.nickname || "you"}`} alt="" />
    <div className="composer-body"><textarea value={content} maxLength={2000} rows={expanded ? 6 : 2} placeholder="分享此刻的想法…" onChange={(event) => setContent(event.target.value)} />
      {previews.length > 0 && <div className={`image-grid count-${previews.length}`}>{previews.map(({ file, url }, index) => <figure key={`${file.name}-${index}`}><img src={url} alt="待上传预览" /><button aria-label="移除图片" onClick={() => setFiles((current) => current.filter((_, itemIndex) => itemIndex !== index))}>×</button></figure>)}</div>}
      <div className="composer-footer"><div><button className="icon-text" onClick={() => inputRef.current?.click()}>▧ <span>图片</span></button><input ref={inputRef} hidden type="file" accept="image/jpeg,image/png,image/webp" multiple onChange={selectFiles} /><small>{content.length}/2000 · {files.length}/9</small></div><button className="primary-button" disabled={!content.trim() || busy} onClick={submit}>{busy ? "发布中…" : "发布"}</button></div>
      {error && <p className="form-error">{error}</p>}
    </div>
  </section>;
}
