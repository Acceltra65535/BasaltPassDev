package notification

import (
	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"
	"errors"
	"time"
)

// Send 发送通知（通用）；receiverIDs 为空表示广播（receiver_id=0）
func Send(appName, title, content, nType string, senderID *uint, senderName string, receiverIDs []uint) error {
	db := common.DB()
	var app model.SystemApp
	if err := db.Where("name = ?", appName).First(&app).Error; err != nil {
		return err
	}

	// 获取发送者的租户关系（如果指定了senderID）
	var senderTenantID uint
	if senderID != nil && *senderID > 0 {
		var sender model.User
		if err := db.Select("id", "is_system_admin").First(&sender, *senderID).Error; err != nil {
			return err
		}
		var senderMembership model.TenantUser
		if !sender.IsSuperAdmin() {
			if err := db.Select("tenant_id").Where("user_id = ?", *senderID).Order("created_at ASC").First(&senderMembership).Error; err != nil {
				return err
			}
			senderTenantID = senderMembership.TenantID
		}

		// 系统管理员可以给任意用户发送通知；租户管理员仅限本租户用户。
		if !sender.IsSuperAdmin() && len(receiverIDs) > 0 && receiverIDs[0] != 0 {
			var count int64
			if err := db.Model(&model.TenantUser{}).
				Where("user_id IN ? AND tenant_id = ?", receiverIDs, senderTenantID).
				Count(&count).Error; err != nil {
				return err
			}
			if int(count) != len(receiverIDs) {
				return errors.New("只能给同一租户的用户发送通知")
			}
		}
	}

	if len(receiverIDs) == 0 {
		receiverIDs = []uint{0}
	}
	now := time.Now()
	notifs := make([]model.Notification, 0, len(receiverIDs))
	for _, rid := range receiverIDs {
		notifs = append(notifs, model.Notification{AppID: &app.ID, Title: title, Content: content, Type: nType, SenderID: senderID, SenderName: senderName, ReceiverID: rid, CreatedAt: now})
	}
	return db.Create(&notifs).Error
}
