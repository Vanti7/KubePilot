package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/api/middleware"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	store     *store.Store
	jwtSecret string
	logger    *zap.Logger
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(s *store.Store, jwtSecret string, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{store: s, jwtSecret: jwtSecret, logger: logger}
}

type loginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type tokenResponse struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expires_at"`
	User      models.User `json:"user"`
}

// Login authenticates a user and returns a JWT.
// POST /auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	result := h.store.DB.WithContext(c.Request.Context()).
		Where("email = ? AND is_active = true", req.Email).
		First(&user)
	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	now := time.Now()
	h.store.DB.WithContext(c.Request.Context()).
		Model(&user).
		Update("last_login_at", now)

	token, expiresAt, err := h.generateToken(user)
	if err != nil {
		h.logger.Error("failed to generate token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, tokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      user,
	})
}

// RefreshToken issues a new JWT for the currently authenticated user.
// POST /auth/refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	userID, _ := c.Get(middleware.ContextKeyUserID)

	var user models.User
	result := h.store.DB.WithContext(c.Request.Context()).
		Where("id = ? AND is_active = true", userID).
		First(&user)
	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	token, expiresAt, err := h.generateToken(user)
	if err != nil {
		h.logger.Error("failed to generate token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, tokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      user,
	})
}

// Me returns the currently authenticated user's profile.
// GET /auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	userID, _ := c.Get(middleware.ContextKeyUserID)

	var user models.User
	result := h.store.DB.WithContext(c.Request.Context()).
		Where("id = ?", userID).
		First(&user)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

type createUserRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Name     string `json:"name"     binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role"`
}

// CreateUser creates a new user account (admin only).
// POST /auth/users
func (h *AuthHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := req.Role
	if role == "" {
		role = models.RoleViewer
	}
	if role != models.RoleAdmin && role != models.RoleOperator && role != models.RoleViewer {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user := models.User{
		ID:           uuid.New(),
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: string(hash),
		Role:         role,
		IsActive:     true,
	}

	if err := h.store.DB.WithContext(c.Request.Context()).Create(&user).Error; err != nil {
		h.logger.Error("failed to create user", zap.Error(err))
		c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
		return
	}

	c.JSON(http.StatusCreated, user)
}

// ListUsers returns all user accounts (admin only).
// GET /auth/users
func (h *AuthHandler) ListUsers(c *gin.Context) {
	var users []models.User
	if err := h.store.DB.WithContext(c.Request.Context()).Order("created_at ASC").Find(&users).Error; err != nil {
		h.logger.Error("list users", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}
	c.JSON(http.StatusOK, users)
}

type updateUserRequest struct {
	Role     *string `json:"role"`
	IsActive *bool   `json:"is_active"`
}

// UpdateUser changes a user's role and/or active state (admin only). Guards
// against demoting or deactivating the last remaining active admin.
// PATCH /auth/users/:id
func (h *AuthHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	callerID, _ := c.Get(middleware.ContextKeyUserID)

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := h.store.DB.WithContext(c.Request.Context()).Where("id = ?", id).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// Determine the resulting state to validate against the last-admin rule.
	newRole := user.Role
	if req.Role != nil {
		if *req.Role != models.RoleAdmin && *req.Role != models.RoleOperator && *req.Role != models.RoleViewer {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
			return
		}
		newRole = *req.Role
	}
	newActive := user.IsActive
	if req.IsActive != nil {
		newActive = *req.IsActive
	}

	losesAdmin := user.Role == models.RoleAdmin && (newRole != models.RoleAdmin || !newActive)
	if losesAdmin && h.countActiveAdmins(c) <= 1 {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot demote or deactivate the last active admin"})
		return
	}
	if user.ID.String() == callerID && !newActive {
		c.JSON(http.StatusConflict, gin.H{"error": "you cannot deactivate your own account"})
		return
	}

	updates := map[string]any{"role": newRole, "is_active": newActive}
	if err := h.store.DB.WithContext(c.Request.Context()).Model(&user).Updates(updates).Error; err != nil {
		h.logger.Error("update user", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// DeleteUser removes a user account (admin only). Guards against deleting the
// caller's own account or the last active admin.
// DELETE /auth/users/:id
func (h *AuthHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	callerID, _ := c.Get(middleware.ContextKeyUserID)
	if id == callerID {
		c.JSON(http.StatusConflict, gin.H{"error": "you cannot delete your own account"})
		return
	}

	var user models.User
	if err := h.store.DB.WithContext(c.Request.Context()).Where("id = ?", id).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if user.Role == models.RoleAdmin && user.IsActive && h.countActiveAdmins(c) <= 1 {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot delete the last active admin"})
		return
	}

	if err := h.store.DB.WithContext(c.Request.Context()).Delete(&models.User{}, "id = ?", id).Error; err != nil {
		h.logger.Error("delete user", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}
	c.Status(http.StatusNoContent)
}

// countActiveAdmins returns the number of active admin accounts.
func (h *AuthHandler) countActiveAdmins(c *gin.Context) int64 {
	var n int64
	h.store.DB.WithContext(c.Request.Context()).
		Model(&models.User{}).
		Where("role = ? AND is_active = ?", models.RoleAdmin, true).
		Count(&n)
	return n
}

type setupRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Name     string `json:"name"     binding:"required"`
}

// Setup creates the first admin account.
// Only available when no users exist in the database (first-run).
// POST /auth/setup
func (h *AuthHandler) Setup(c *gin.Context) {
	var count int64
	if err := h.store.DB.WithContext(c.Request.Context()).Model(&models.User{}).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "setup already completed"})
		return
	}

	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user := models.User{
		ID:           uuid.New(),
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: string(hash),
		Role:         models.RoleAdmin,
		IsActive:     true,
	}
	if err := h.store.DB.WithContext(c.Request.Context()).Create(&user).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "failed to create admin user"})
		return
	}

	h.logger.Info("admin user created via setup endpoint", zap.String("email", user.Email))

	token, expiresAt, err := h.generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusCreated, tokenResponse{Token: token, ExpiresAt: expiresAt, User: user})
}

// SetupStatus returns whether first-run setup is needed.
// GET /auth/setup
func (h *AuthHandler) SetupStatus(c *gin.Context) {
	var count int64
	h.store.DB.WithContext(c.Request.Context()).Model(&models.User{}).Count(&count)
	c.JSON(http.StatusOK, gin.H{"setup_required": count == 0})
}

// generateToken creates a signed JWT for the given user.
func (h *AuthHandler) generateToken(user models.User) (string, time.Time, error) {
	expiresAt := time.Now().Add(24 * time.Hour)

	claims := middleware.Claims{
		UserID: user.ID.String(),
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}
