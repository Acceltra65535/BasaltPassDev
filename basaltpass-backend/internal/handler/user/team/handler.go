package team

import (
	userdto "basaltpass-backend/internal/dto/user"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// 全局处理器实例（用户侧）
var (
	userTeamService *Service
	userTeamHandler *Handler
)

// InitHandler 初始化团队处理器（用户侧）
func InitHandler(db *gorm.DB) {
	userTeamService = NewService(db)
	userTeamHandler = NewHandler(userTeamService)
}

// 用户侧导出函数，供路由使用
func CreateTeamHandler(c *fiber.Ctx) error     { return userTeamHandler.CreateTeam(c) }
func GetUserTeamsHandler(c *fiber.Ctx) error   { return userTeamHandler.GetUserTeams(c) }
func GetTeamHandler(c *fiber.Ctx) error        { return userTeamHandler.GetTeam(c) }
func UpdateTeamHandler(c *fiber.Ctx) error     { return userTeamHandler.UpdateTeam(c) }
func DeleteTeamHandler(c *fiber.Ctx) error     { return userTeamHandler.DeleteTeam(c) }
func GetTeamMembersHandler(c *fiber.Ctx) error { return userTeamHandler.GetTeamMembers(c) }
func GetTeamWalletsHandler(c *fiber.Ctx) error { return userTeamHandler.GetTeamWallets(c) }
func GetTeamWalletHistoryHandler(c *fiber.Ctx) error {
	return userTeamHandler.GetTeamWalletHistory(c)
}
func AddMemberHandler(c *fiber.Ctx) error        { return userTeamHandler.AddMember(c) }
func UpdateMemberRoleHandler(c *fiber.Ctx) error { return userTeamHandler.UpdateMemberRole(c) }
func RemoveMemberHandler(c *fiber.Ctx) error     { return userTeamHandler.RemoveMember(c) }
func LeaveTeamHandler(c *fiber.Ctx) error        { return userTeamHandler.LeaveTeam(c) }

// CreateTeam 创建团队
func (h *Handler) CreateTeam(c *fiber.Ctx) error {
	var req userdto.CreateTeamRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数错误"})
	}
	userID := c.Locals("userID").(uint)
	team, err := h.service.CreateTeam(userID, &req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"data":    fiber.Map{"team": team},
		"message": "团队创建成功",
	})
}

// GetTeam 获取团队信息
func (h *Handler) GetTeam(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "团队ID无效"})
	}
	userID := c.Locals("userID").(uint)
	team, err := h.service.GetTeam(uint(teamID), userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": team})
}

// UpdateTeam 更新团队信息
func (h *Handler) UpdateTeam(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "团队ID无效"})
	}
	var req userdto.UpdateTeamRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数错误"})
	}
	userID := c.Locals("userID").(uint)
	if err := h.service.UpdateTeam(uint(teamID), userID, &req); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "团队信息更新成功"})
}

// DeleteTeam 删除团队
func (h *Handler) DeleteTeam(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "团队ID无效"})
	}
	userID := c.Locals("userID").(uint)
	if err := h.service.DeleteTeam(uint(teamID), userID); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "团队删除成功"})
}

// GetUserTeams 获取用户的所有团队
func (h *Handler) GetUserTeams(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)
	teams, err := h.service.GetUserTeams(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": teams})
}

// AddMember 添加团队成员
func (h *Handler) AddMember(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "团队ID无效"})
	}
	var req userdto.AddMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数错误"})
	}
	userID := c.Locals("userID").(uint)
	if err := h.service.AddMember(uint(teamID), userID, &req); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "成员添加成功"})
}

// UpdateMemberRole 更新成员角色
func (h *Handler) UpdateMemberRole(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "团队ID无效"})
	}
	memberID, err := strconv.ParseUint(c.Params("member_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "成员ID无效"})
	}
	var req userdto.UpdateMemberRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数错误"})
	}
	userID := c.Locals("userID").(uint)
	if err := h.service.UpdateMemberRole(uint(teamID), userID, uint(memberID), &req); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "成员角色更新成功"})
}

// RemoveMember 移除团队成员
func (h *Handler) RemoveMember(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "团队ID无效"})
	}
	memberID, err := strconv.ParseUint(c.Params("member_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "成员ID无效"})
	}
	userID := c.Locals("userID").(uint)
	if err := h.service.RemoveMember(uint(teamID), userID, uint(memberID)); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "成员移除成功"})
}

// GetTeamMembers 获取团队成员列表
func (h *Handler) GetTeamMembers(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "团队ID无效"})
	}
	userID := c.Locals("userID").(uint)
	members, err := h.service.GetTeamMembers(uint(teamID), userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": members})
}

func (h *Handler) GetTeamWallets(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "团队ID无效"})
	}
	userID := c.Locals("userID").(uint)
	wallets, err := h.service.GetTeamWallets(uint(teamID), userID)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	data := make([]fiber.Map, 0, len(wallets))
	for _, wallet := range wallets {
		data = append(data, fiber.Map{
			"id": wallet.ID, "tenant_id": wallet.TenantID,
			"owner_type": wallet.OwnerType, "owner_id": wallet.OwnerID,
			"currency_id": wallet.CurrencyID, "balance": wallet.Balance,
			"freeze": wallet.Freeze, "currency": wallet.Currency,
			"created_at": wallet.CreatedAt, "updated_at": wallet.UpdatedAt,
		})
	}
	return c.JSON(fiber.Map{"data": data})
}

func (h *Handler) GetTeamWalletHistory(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "团队ID无效"})
	}
	currencyCode := c.Query("currency")
	if currencyCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "currency is required"})
	}
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	userID := c.Locals("userID").(uint)
	txs, err := h.service.GetTeamWalletHistory(uint(teamID), userID, currencyCode, limit)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	data := make([]fiber.Map, 0, len(txs))
	for _, tx := range txs {
		data = append(data, fiber.Map{
			"id": tx.ID, "wallet_id": tx.WalletID, "type": tx.Type,
			"amount": tx.Amount, "status": tx.Status, "reference": tx.Reference,
			"created_at": tx.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{"data": data})
}

// LeaveTeam 离开团队
func (h *Handler) LeaveTeam(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "团队ID无效"})
	}
	userID := c.Locals("userID").(uint)
	if err := h.service.LeaveTeam(uint(teamID), userID); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "已成功离开团队"})
}
