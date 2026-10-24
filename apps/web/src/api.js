import { mockApi } from "./mock.js";

export const useMock = import.meta.env.VITE_USE_MOCK === "true";
const baseURL = (import.meta.env.VITE_API_BASE_URL || "").replace(/\/$/, "");

async function request(path, { token, method = "POST", body, headers } = {}) {
  const response = await fetch(`${baseURL}${path}`, {
    method,
    headers: { ...(body === undefined ? {} : { "Content-Type": "application/json" }), ...(token ? { Authorization: `Bearer ${token}` } : {}), ...headers },
    body: body === undefined ? undefined : JSON.stringify(body)
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    const error = new Error(payload.error || `请求失败（${response.status}）`);
    error.status = response.status;
    throw error;
  }
  return response.status === 204 ? null : response.json();
}

async function upload(file, token) {
  const form = new FormData();
  form.append("file", file);
  form.append("kind", "post");
  const response = await fetch(`${baseURL}/api/uploads/create`, { method: "POST", headers: { Authorization: `Bearer ${token}` }, body: form });
  if (!response.ok) throw new Error("图片上传失败");
  return response.json();
}

const realApi = {
  login: (body) => request("/api/sessions/login", { body }),
  register: (body) => request("/api/users/register", { body }),
  me: (token) => request("/api/users/me/detail", { token, body: {} }),
  profile: (id) => request("/api/users/detail", { body: { user_id: Number(id) } }),
  updateProfile: (body, token) => request("/api/users/me/update", { token, body }),
  followingFeed: (token) => request("/api/feed/following", { token, body: { limit: 20 } }),
  hotFeed: (token) => request("/api/feed/hot", { token, body: { limit: 20, offset: 0 } }),
  myPosts: (token) => request("/api/posts/list-mine", { token, body: { limit: 50, offset: 0 } }),
  userPosts: (id) => request("/api/posts/list", { body: { author_id: Number(id), limit: 50, offset: 0 } }),
  upload,
  createPost: (body, token) => request("/api/posts/create", { token, body, headers: { "Idempotency-Key": crypto.randomUUID() } }),
  setAction: (postId, type, active, token) => request(`/api/posts/${active ? type : `un${type}`}`, { token, body: { post_id: postId }, headers: { "Idempotency-Key": crypto.randomUUID() } }),
  listComments: (postId) => request("/api/posts/comments/list", { body: { post_id: postId, limit: 50, offset: 0 } }),
  createComment: (postId, content, token) => request("/api/posts/comments/create", { token, body: { post_id: postId, content }, headers: { "Idempotency-Key": crypto.randomUUID() } }),
  messages: (token) => request("/api/messages/list", { token, body: { limit: 50 } }),
  unread: (token) => request("/api/messages/unread-count", { token, body: {} }),
  markRead: (ids, token) => request("/api/messages/read", { token, body: { message_ids: ids } }),
  follow: (id, active, token) => request(`/api/relations/${active ? "follow" : "unfollow"}`, { token, body: { target_user_id: Number(id) }, headers: { "Idempotency-Key": crypto.randomUUID() } }),
  logout: (token) => request("/api/sessions/logout", { token, body: {} })
};

export const api = useMock ? mockApi : realApi;
export const assetURL = (value) => !value || /^(https?:|blob:|data:)/.test(value) ? value : `${baseURL}${value}`;
