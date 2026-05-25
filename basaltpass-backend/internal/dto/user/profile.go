package user

// ProfileResponse represents public user profile fields.
type ProfileResponse struct {
	ID           uint   `json:"id"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Nickname     string `json:"nickname"`
	NickName     string `json:"nick_name"`
	GivenName    string `json:"given_name"`
	FamilyName   string `json:"family_name"`
	MiddleName   string `json:"middle_name"`
	Locale       string `json:"locale"`
	Zoneinfo     string `json:"zoneinfo"`
	AvatarURL    string `json:"avatar_url"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	HasTenant    bool   `json:"has_tenant"`
	TenantID     *uint  `json:"tenant_id,omitempty"`
	TenantRole   string `json:"tenant_role,omitempty"`
	Banned       bool   `json:"banned"`
}

// UpdateProfileRequest defines fields that can be updated.
type UpdateProfileRequest struct {
	Nickname   *string `json:"nickname,omitempty"`
	GivenName  *string `json:"given_name,omitempty"`
	FamilyName *string `json:"family_name,omitempty"`
	MiddleName *string `json:"middle_name,omitempty"`
	Locale     *string `json:"locale,omitempty"`
	Zoneinfo   *string `json:"zoneinfo,omitempty"`
	Email      *string `json:"email,omitempty"`
	Phone      *string `json:"phone,omitempty"`
	AvatarURL  *string `json:"avatar_url,omitempty"`
}

// UserSearchResult represents user search result
type UserSearchResult struct {
	ID       uint   `json:"id"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar,omitempty"`
}
