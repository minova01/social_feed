const now = Date.now();
const avatars = ["https://i.pravatar.cc/160?img=47", "https://i.pravatar.cc/160?img=12", "https://i.pravatar.cc/160?img=32"];
const photos = [
  "https://images.unsplash.com/photo-1500530855697-b586d89ba3ee?auto=format&fit=crop&w=1200&q=85",
  "https://images.unsplash.com/photo-1511818966892-d7d671e672a2?auto=format&fit=crop&w=900&q=85",
  "https://images.unsplash.com/photo-1495474472287-4d71bcdd2085?auto=format&fit=crop&w=900&q=85"
];

let currentUser = { id: 1, account: "mori", nickname: "林默", avatar_url: avatars[0], bio: "记录设计、城市和缓慢发生的事。", following_count: 128, follower_count: 18640, work_count: 3 };
let posts = [
  { post_id: 101, author_id: 2, author_nickname: "Nora", author_avatar_url: avatars[1], content: "清晨的城市还没决定今天要成为什么样子。路过旧车站，光刚好落在墙面上。", image_urls: [photos[0], photos[1]], like_count: 1284, comment_count: 48, favorite_count: 209, liked: false, favorited: true, published_at: now - 18 * 60_000 },
  { post_id: 102, author_id: 3, author_nickname: "周屿", author_avatar_url: avatars[2], content: "最近重新整理了工作桌。真正让人平静的不是空无一物，而是每件东西都有自己的位置。", image_urls: [photos[2]], like_count: 846, comment_count: 31, favorite_count: 122, liked: true, favorited: false, published_at: now - 96 * 60_000 },
  { post_id: 103, author_id: 1, author_nickname: "林默", author_avatar_url: avatars[0], content: "把复杂的事情做轻，把重要的事情做久。", image_urls: [], like_count: 392, comment_count: 16, favorite_count: 88, liked: false, favorited: false, published_at: now - 5 * 60 * 60_000 }
];
let comments = { 101: [{ id: 1, post_id: 101, user_id: 3, user_nickname: "周屿", user_avatar_url: avatars[2], content: "第二张的光影很好。", created_at: now - 8 * 60_000 }] };
let messages = [
  { id: 1, type: "like", title: "Nora 赞了你的帖子", content: "把复杂的事情做轻…", actor_id: 2, actor_nickname: "Nora", actor_avatar_url: avatars[1], is_read: false, created_at: now - 12 * 60_000 },
  { id: 2, type: "follow", title: "周屿关注了你", content: "", actor_id: 3, actor_nickname: "周屿", actor_avatar_url: avatars[2], is_read: true, created_at: now - 2 * 60 * 60_000 }
];

const delay = (value) => new Promise((resolve) => setTimeout(() => resolve(structuredClone(value)), 180));
const profileFor = (id) => id === 1 ? currentUser : { id, nickname: id === 2 ? "Nora" : "周屿", avatar_url: avatars[id === 2 ? 1 : 2], bio: "关注日常，也关注那些没有被说出来的部分。", following_count: 86, follower_count: id === 2 ? 24880 : 7320, work_count: posts.filter((post) => post.author_id === id).length };
const normalizeOwnPost = (post) => ({ ...post, id: post.post_id, author_id: post.author_id, published_at: post.published_at });

export const mockApi = {
  login: async () => delay({ access_token: "mock-access-token" }),
  register: async ({ account, nickname }) => delay({ ...currentUser, account, nickname }),
  me: async () => delay(currentUser),
  profile: async (id) => delay(profileFor(Number(id))),
  updateProfile: async (body) => { currentUser = { ...currentUser, ...body }; return delay(currentUser); },
  followingFeed: async () => delay({ items: posts }),
  hotFeed: async () => delay({ items: [...posts].sort((a, b) => b.like_count - a.like_count) }),
  searchPosts: async (keyword) => delay({ items: posts.filter((post) => post.content.toLocaleLowerCase().includes(keyword.trim().toLocaleLowerCase())), limit: 20, offset: 0 }),
  myPosts: async () => delay({ items: posts.filter((post) => post.author_id === 1).map(normalizeOwnPost) }),
  userPosts: async (id) => delay({ items: posts.filter((post) => post.author_id === Number(id)).map(normalizeOwnPost) }),
  upload: async (file) => delay({ url: URL.createObjectURL(file), kind: "post", filename: file.name, size: file.size }),
  createPost: async ({ content, image_urls }) => {
    const post = { post_id: now + posts.length, author_id: 1, author_nickname: currentUser.nickname, author_avatar_url: currentUser.avatar_url, content, image_urls, like_count: 0, comment_count: 0, favorite_count: 0, liked: false, favorited: false, published_at: Date.now() };
    posts = [post, ...posts];
    return delay(normalizeOwnPost(post));
  },
  setAction: async (postId, type, active) => {
    const post = posts.find((item) => item.post_id === postId);
    const field = type === "like" ? "liked" : "favorited";
    const count = type === "like" ? "like_count" : "favorite_count";
    if (post && post[field] !== active) { post[field] = active; post[count] += active ? 1 : -1; }
    return delay({ post_id: postId, action_type: type, active, like_count: post?.like_count || 0, favorite_count: post?.favorite_count || 0 });
  },
  listComments: async (postId) => delay({ items: comments[postId] || [], limit: 50, offset: 0 }),
  createComment: async (postId, content) => {
    const item = { id: Date.now(), post_id: postId, user_id: 1, user_nickname: currentUser.nickname, user_avatar_url: currentUser.avatar_url, content, created_at: Date.now() };
    comments[postId] = [...(comments[postId] || []), item];
    const post = posts.find((entry) => entry.post_id === postId); if (post) post.comment_count += 1;
    return delay({ ...item, comment_count: post?.comment_count || 1 });
  },
  messages: async () => delay({ items: messages, next_cursor: "", has_more: false }),
  unread: async () => delay({ unread_count: messages.filter((item) => !item.is_read).length }),
  markRead: async (ids) => { messages = messages.map((item) => ids.includes(item.id) ? { ...item, is_read: true } : item); return delay({ updated_count: ids.length }); },
  follow: async (id, active) => delay({ target_user_id: id, following: active }),
  logout: async () => undefined
};
