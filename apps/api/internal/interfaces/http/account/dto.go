package interfaceshttpaccount

// 账号注册请求
type RegisterRequest struct {
	Account  string `json:"account" binding:"required,max=64"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	Nickname string `json:"nickname" binding:"required,max=40"`
}

// 账号登录请求
type LoginByPasswordRequest struct {
	Account  string `json:"account" binding:"required,max=64"`
	Password string `json:"password" binding:"required,max=72"`
}

// 用户资料更新请求
type UpdateProfileRequest struct {
	Nickname  *string `json:"nickname" binding:"omitempty,max=40"`
	AvatarURL *string `json:"avatar_url" binding:"omitempty,max=512"`
	Bio       *string `json:"bio" binding:"omitempty,max=500"`
}

type userDetailRequest struct {
	UserID int64 `json:"user_id" binding:"required,gt=0"`
}

// 账号登录响应
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

// 用户信息响应
type userProfileResponse struct {
	ID             int64  `json:"id"`
	Account        string `json:"account"`
	Nickname       string `json:"nickname"`
	AvatarURL      string `json:"avatar_url"`
	Bio            string `json:"bio"`
	Status         int    `json:"status"`
	Role           string `json:"role"`
	FollowingCount int    `json:"following_count"`
	FollowerCount  int    `json:"follower_count"`
	WorkCount      int    `json:"work_count"`
}

// 公开用户信息响应，隐藏账号、角色和状态等内部字段。
type publicUserProfileResponse struct {
	ID             int64  `json:"id"`
	Nickname       string `json:"nickname"`
	AvatarURL      string `json:"avatar_url"`
	Bio            string `json:"bio"`
	FollowingCount int    `json:"following_count"`
	FollowerCount  int    `json:"follower_count"`
	WorkCount      int    `json:"work_count"`
}
