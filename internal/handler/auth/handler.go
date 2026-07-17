package auth

import (
	"golang.org/x/crypto/bcrypt"
	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/jwtutil"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
	"net/http"
	"sync"
	"time"
)

// loginAttempt 登录尝试记录
type loginAttempt struct {
	count        int
	lastAttempt  time.Time
}

// loginFailures 登录失败计数器（key=username, value=*loginAttempt）
var loginFailures sync.Map

const (
	maxLoginFailures    = 5
	loginLockoutMinutes = 15
)

// LoginRequest 登录请求参数
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginHandler 用户登录
func LoginHandler(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// 检查账户是否被锁定
	if val, ok := loginFailures.Load(req.Username); ok {
		attempt := val.(*loginAttempt)
		if attempt.count >= maxLoginFailures && time.Since(attempt.lastAttempt) < loginLockoutMinutes*time.Minute {
			c.JSON(http.StatusTooManyRequests, gin.H{"message": "账户已锁定，请 15 分钟后再试"})
			c.Abort()
			return
		}
	}

	dbVal, exists := c.Get("db")
	if !exists || dbVal == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	db := dbVal.(*store.DB)

	user, err := db.GetUserByUsername(c.Request.Context(), req.Username)
	if err != nil {
		recordLoginFailure(req.Username)
		response.Unauthorized(c, "用户名或密码错误")
		return
	}

	// bcrypt 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		recordLoginFailure(req.Username)
		response.Unauthorized(c, "用户名或密码错误")
		return
	}

	// 登录成功，清除失败计数
	loginFailures.Delete(req.Username)

	jwtSecret, _ := c.Get("jwtSecret")
	secretStr, _ := jwtSecret.(string)

	expireIn := time.Now().Add(24 * time.Hour).Unix()
	token, err := jwtutil.GenerateToken(user, secretStr, expireIn)
	if err != nil {
		response.InternalServerError(c, "生成 Token 失败")
		return
	}

	response.Success(c, gin.H{
		"token": token,
		"user": gin.H{
			"id":        user.ID,
			"username":  user.Username,
			"role":      user.Role,
			"name":      user.Name,
			"family_id": user.FamilyID,
		},
	})
}

// recordLoginFailure 记录一次登录失败
func recordLoginFailure(username string) {
	val, ok := loginFailures.Load(username)
	if !ok {
		loginFailures.Store(username, &loginAttempt{count: 1, lastAttempt: time.Now()})
		return
	}
	attempt := val.(*loginAttempt)
	// 如果锁定时间已过，重置计数
	if time.Since(attempt.lastAttempt) >= loginLockoutMinutes*time.Minute {
		attempt.count = 1
	} else {
		attempt.count++
	}
	attempt.lastAttempt = time.Now()
}

// RegisterRequest 注册请求参数
type RegisterRequest struct {
	Username string     `json:"username" binding:"required"`
	Password string     `json:"password" binding:"required,min=6"`
	Name     string     `json:"name" binding:"required"`
	Role     model.Role `json:"role" binding:"required"`
}

// RegisterHandler 注册用户（仅 admin 可访问）
func RegisterHandler(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	dbVal, exists := c.Get("db")
	if !exists || dbVal == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	db := dbVal.(*store.DB)

	// 检查用户名是否已存在
	if _, err := db.GetUserByUsername(c.Request.Context(), req.Username); err == nil {
		response.BadRequest(c, "用户名已存在")
		return
	}

	// bcrypt 加密密码
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		response.InternalServerError(c, "密码加密失败")
		return
	}

	user := &model.User{
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         req.Role,
		Name:         req.Name,
		FamilyID:     1,
	}

	id, err := db.CreateUser(c.Request.Context(), user)
	if err != nil {
		response.InternalServerError(c, "创建用户失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"id":       id,
		"username": req.Username,
		"role":     req.Role,
		"name":     req.Name,
	})
}

// HashPassword 对密码进行 bcrypt 加密
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword 验证密码
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}