package member

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/pkg/response"
)

// AvatarDir 头像上传目录（对应 web/assets/avatars，静态资源由 StaticFS 暴露）
const AvatarDir = "web/assets/avatars"

// AvatarMaxSize 头像最大 5MB
const AvatarMaxSize = 5 << 20

// avatarExtensions 允许的图片扩展名
var avatarExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
}

// sanitizeAvatarName 文件名安全化：保留中文，去除路径分隔符与特殊字符
func sanitizeAvatarName(name string) string {
	s := strings.TrimSpace(name)
	s = strings.ReplaceAll(s, `/`, "_")
	s = strings.ReplaceAll(s, `\`, "_")
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, "*", "_")
	s = strings.ReplaceAll(s, "?", "_")
	s = strings.ReplaceAll(s, `"`, "_")
	s = strings.ReplaceAll(s, "<", "_")
	s = strings.ReplaceAll(s, ">", "_")
	s = strings.ReplaceAll(s, "|", "_")
	return s
}

// UploadAvatarHandler 上传成员头像
// 照片保存为 web/assets/avatars/{用户名}.{ext}，命名规则与本地静态文件方案一致
// 返回 avatar_url，前端拿到后写入 memberFormAvatar
//
// POST /api/members/avatar
// form: file=<图片>, name=<用户名>
func UploadAvatarHandler(c *gin.Context) {
	// 限制大小
	if err := c.Request.ParseMultipartForm(AvatarMaxSize); err != nil {
		response.BadRequest(c, "文件过大，头像最大支持 5MB")
		return
	}

	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		response.BadRequest(c, "缺少成员名称")
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择要上传的头像图片")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !avatarExtensions[ext] {
		response.BadRequest(c, "仅支持 jpg/jpeg/png/webp 格式")
		return
	}

	// 确保目录存在
	if err := os.MkdirAll(AvatarDir, 0755); err != nil {
		response.Error(c, 500, "创建头像目录失败")
		return
	}

	// 文件名 = 用户名 + 扩展名（保留中文）
	safeName := sanitizeAvatarName(name)
	if safeName == "" {
		// 名称全部是特殊字符的兜底：用随机串
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		safeName = "member_" + hex.EncodeToString(b)
	}
	finalPath := filepath.Join(AvatarDir, safeName+ext)

	// 写入文件
	if err := c.SaveUploadedFile(header, finalPath); err != nil {
		response.Error(c, 500, "保存头像失败: "+err.Error())
		return
	}

	// 返回可访问 URL（前端按此 URL 加载，可直接命中静态文件路由）
	avatarURL := fmt.Sprintf("/assets/avatars/%s%s", safeName, ext)

	response.Success(c, gin.H{
		"avatar_url": avatarURL,
		"file_name":  safeName + ext,
		"size":       header.Size,
	})
}
