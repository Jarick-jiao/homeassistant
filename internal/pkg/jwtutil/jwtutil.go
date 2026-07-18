package jwtutil

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/homemate/server/internal/model"
)

// GenerateToken 根据用户信息生成 JWT Token
// v3.6.0: 新增 isAdmin 参数，标记系统管理员（admin 账号或被提升的成员）
func GenerateToken(user *model.User, secret string, expireIn int64, isAdmin bool) (string, error) {
	claims := jwt.MapClaims{
		"user_id":   user.ID,
		"username":  user.Username,
		"role":      string(user.Role),
		"family_id": user.FamilyID,
		"is_admin":  isAdmin,
		"exp":       expireIn,
		"iat":       expireIn - 86400, // now
		"iss":       "homemate",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken 解析 JWT Token
func ParseToken(tokenString string, secret string) (*model.Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signature")
		}
		return []byte(secret), nil
	})
	if err != nil {
		// 区分 token 过期和其他签名/解析错误
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("token expired")
		}
		return nil, errors.New("invalid signature")
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		c := &model.Claims{}
		if v, ok := claims["user_id"].(float64); ok {
			c.UserID = int64(v)
		}
		if v, ok := claims["username"].(string); ok {
			c.Username = v
		}
		if v, ok := claims["role"].(string); ok {
			c.Role = model.Role(v)
		}
		if v, ok := claims["family_id"].(float64); ok {
			c.FamilyID = int64(v)
		}
		// v3.6.0: 兼容旧 token（无 is_admin 字段时默认 false）
		if v, ok := claims["is_admin"].(bool); ok {
			c.IsAdmin = v
		}
		return c, nil
	}
	return nil, errors.New("invalid claims")
}
