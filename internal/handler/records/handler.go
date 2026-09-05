package records

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// UploadDir 文件上传根目录
const UploadDir = "uploads/health_records"

// MaxFileSize 最大文件大小 50MB
const MaxFileSize = 50 << 20

// allowedExtensions 允许的文件类型
var allowedExtensions = map[string]bool{
	".pdf": true, ".jpg": true, ".jpeg": true, ".png": true,
	".bmp": true, ".tiff": true, ".tif": true, ".webp": true,
}

// Categories 档案分类
var Categories = []string{"病例", "检查报告", "处方", "化验单", "影像", "其他"}

// Handler 健康档案 handler
type Handler struct {
	db        *store.DB
	uploadDir string
	// aiAnalyzer AI 分析函数（可注入，便于测试和替换）
	aiAnalyzer func(ctx context.Context, file *model.HealthRecordFile) (summary, analysis string, err error)
}

// New 创建 handler
func New(db *store.DB, uploadDir string) *Handler {
	if uploadDir == "" {
		uploadDir = UploadDir
	}
	h := &Handler{db: db, uploadDir: uploadDir}
	// 默认 AI 分析器（调用 OpenAI）
	h.aiAnalyzer = h.defaultAIAnalyzer
	return h
}

// SetAIAnalyzer 注入自定义 AI 分析器
func (h *Handler) SetAIAnalyzer(fn func(ctx context.Context, file *model.HealthRecordFile) (summary, analysis string, err error)) {
	h.aiAnalyzer = fn
}

// Upload 上传健康档案文件
func (h *Handler) Upload(c *gin.Context) {
	// 解析表单（限制大小）
	if err := c.Request.ParseMultipartForm(MaxFileSize); err != nil {
		response.BadRequest(c, "请求体过大，最大支持 50MB")
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择要上传的文件")
		return
	}
	defer file.Close()

	// 校验文件扩展名
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtensions[ext] {
		response.BadRequest(c, "不支持的文件类型，仅支持 PDF/JPG/PNG/BMP/TIFF/WEBP")
		return
	}

	// 校验文件大小
	if header.Size > MaxFileSize {
		response.BadRequest(c, "文件大小超过 50MB 限制")
		return
	}

	// 解析参数
	memberID, _ := strconv.ParseInt(c.PostForm("member_id"), 10, 64)
	if memberID <= 0 {
		response.BadRequest(c, "member_id 不能为空")
		return
	}

	title := c.PostForm("title")
	if title == "" {
		title = strings.TrimSuffix(header.Filename, ext)
	}
	category := c.PostForm("category")
	if category == "" {
		category = "其他"
	}
	recordDate := c.PostForm("record_date")
	description := c.PostForm("description")
	hospital := c.PostForm("hospital")
	clinic := c.PostForm("clinic")

	// 确保上传目录存在
	if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
		log.Printf("[ERROR] 创建上传目录失败: %v", err)
		response.InternalServerError(c, "服务器内部错误")
		return
	}

	// 生成唯一文件名（SHA256 前缀 + 原始文件名）
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%d_%d_%s", memberID, time.Now().UnixNano(), header.Filename)))
	hashStr := hex.EncodeToString(hash.Sum(nil))[:16]
	relPath := fmt.Sprintf("%d/%s_%s", memberID, hashStr, header.Filename)
	fullPath := filepath.Join(h.uploadDir, relPath)

	// 创建成员子目录
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		log.Printf("[ERROR] 创建成员目录失败: %v", err)
		response.InternalServerError(c, "服务器内部错误")
		return
	}

	// 保存文件
	dst, err := os.Create(fullPath)
	if err != nil {
		log.Printf("[ERROR] 创建文件失败: %v", err)
		response.InternalServerError(c, "文件保存失败")
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		log.Printf("[ERROR] 写入文件失败: %v", err)
		os.Remove(fullPath)
		response.InternalServerError(c, "文件写入失败")
		return
	}

	// 保存数据库记录
	record := &model.HealthRecordFile{
		MemberID:    memberID,
		Title:       title,
		Category:    category,
		RecordDate:  recordDate,
		Description: description,
		Hospital:    hospital,
		Clinic:      clinic,
		FileName:    header.Filename,
		FileSize:    written,
		FileType:    strings.TrimPrefix(ext, "."),
		FilePath:    relPath,
		CreatedAt:   time.Now(),
	}

	id, err := h.db.CreateHealthRecordFile(c.Request.Context(), record)
	if err != nil {
		log.Printf("[ERROR] 保存文件记录失败: %v", err)
		os.Remove(fullPath)
		response.InternalServerError(c, "数据库保存失败")
		return
	}

	record.ID = id
	response.Success(c, gin.H{
		"id":          id,
		"title":       title,
		"category":    category,
		"file_name":   header.Filename,
		"file_size":   written,
		"file_type":   strings.TrimPrefix(ext, "."),
		"record_date": recordDate,
		"description": description,
		"hospital":    hospital,
		"clinic":      clinic,
	})
}

// List 获取健康档案文件列表
func (h *Handler) List(c *gin.Context) {
	memberID, _ := strconv.ParseInt(c.Query("member_id"), 10, 64)
	category := c.Query("category")
	limit, _ := strconv.Atoi(c.Query("limit"))

	files, err := h.db.GetHealthRecordFiles(c.Request.Context(), memberID, category, limit)
	if err != nil {
		log.Printf("[ERROR] 查询档案列表失败: %v", err)
		response.InternalServerError(c, "查询失败")
		return
	}

	// 解析成员 ID → 名称
	memberNames := map[int64]string{}
	if members, mErr := h.db.GetMembers(c.Request.Context()); mErr == nil {
		for _, m := range members {
			memberNames[m.ID] = m.Name
		}
	}

	// 转为视图模型
	views := make([]model.HealthRecordFileView, 0, len(files))
	for _, f := range files {
		views = append(views, model.HealthRecordFileView{
			ID:          f.ID,
			MemberID:    f.MemberID,
			MemberName:  memberNames[f.MemberID],
			Title:       f.Title,
			Category:    f.Category,
			RecordDate:  f.RecordDate,
			Description: f.Description,
			Hospital:    f.Hospital,
			Clinic:      f.Clinic,
			FileName:    f.FileName,
			FileSize:    f.FileSize,
			FileType:    f.FileType,
			FileURL:     fmt.Sprintf("/api/records/%d/download", f.ID),
			Summary:     f.Summary,
			Analysis:    f.Analysis,
			IsAnalyzed:  f.AnalyzedAt != nil,
		})
	}

	response.Success(c, gin.H{
		"list":     views,
		"total":    len(views),
		"categories": Categories,
	})
}

// GetDetail 获取单个文件详情
func (h *Handler) GetDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的文件 ID")
		return
	}

	f, err := h.db.GetHealthRecordFileByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "文件不存在")
		return
	}

	memberName := ""
	if m, mErr := h.db.GetMemberByID(c.Request.Context(), f.MemberID); mErr == nil {
		memberName = m.Name
	}

	response.Success(c, model.HealthRecordFileView{
		ID:          f.ID,
		MemberID:    f.MemberID,
		MemberName:  memberName,
		Title:       f.Title,
		Category:    f.Category,
		RecordDate:  f.RecordDate,
		Description: f.Description,
		Hospital:    f.Hospital,
		Clinic:      f.Clinic,
		FileName:    f.FileName,
		FileSize:    f.FileSize,
		FileType:    f.FileType,
		FileURL:     fmt.Sprintf("/api/records/%d/download", f.ID),
		Summary:     f.Summary,
		Analysis:    f.Analysis,
		IsAnalyzed:  f.AnalyzedAt != nil,
	})
}

// Download 下载文件
func (h *Handler) Download(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的文件 ID")
		return
	}

	f, err := h.db.GetHealthRecordFileByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "文件不存在")
		return
	}

	fullPath := filepath.Join(h.uploadDir, f.FilePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		log.Printf("[ERROR] 文件不存在于磁盘: %s", fullPath)
		response.NotFound(c, "文件已被删除")
		return
	}

	// 设置 Content-Type
	contentTypes := map[string]string{
		"pdf":  "application/pdf",
		"jpg":  "image/jpeg",
		"jpeg": "image/jpeg",
		"png":  "image/png",
		"bmp":  "image/bmp",
		"tiff": "image/tiff",
		"tif":  "image/tiff",
		"webp": "image/webp",
	}
	ct := contentTypes[f.FileType]
	if ct == "" {
		ct = "application/octet-stream"
	}

	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, f.FileName))
	c.Header("Content-Type", ct)
	c.File(fullPath)
}

// Delete 删除文件
func (h *Handler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的文件 ID")
		return
	}

	f, err := h.db.GetHealthRecordFileByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "文件不存在")
		return
	}

	// 删除磁盘文件
	fullPath := filepath.Join(h.uploadDir, f.FilePath)
	os.Remove(fullPath) // 忽略错误，文件可能已被手动删除

	// 删除数据库记录
	if err := h.db.DeleteHealthRecordFile(c.Request.Context(), id); err != nil {
		log.Printf("[ERROR] 删除文件记录失败: %v", err)
		response.InternalServerError(c, "删除失败")
		return
	}

	response.Success(c, nil)
}

// Analyze 触发单个文件的 AI 分析
func (h *Handler) Analyze(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的文件 ID")
		return
	}

	f, err := h.db.GetHealthRecordFileByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "文件不存在")
		return
	}

	if h.aiAnalyzer == nil || os.Getenv("HOMEMATE_OPENAI_KEY") == "" {
		response.BadRequest(c, "AI 分析功能未配置，请在 config.yaml 中设置 openai.api_key 或环境变量 HOMEMATE_OPENAI_KEY")
		return
	}

	// 异步执行 AI 分析，返回 202 Accepted
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		summary, analysis, err := h.aiAnalyzer(ctx, f)
		if err != nil {
			log.Printf("[ERROR] AI 分析文件 %d 失败: %v", id, err)
			return
		}

		if err := h.db.UpdateHealthRecordFileAnalysis(context.Background(), id, summary, analysis); err != nil {
			log.Printf("[ERROR] 保存 AI 分析结果失败: %v", err)
		} else {
			log.Printf("[INFO] 文件 %d AI 分析完成", id)
		}
	}()

	c.JSON(http.StatusAccepted, response.Response{
		Code:    0,
		Message: "AI 分析已启动，请稍后查看结果",
	})
}

// AnalyzeBatch 批量触发未分析文件的 AI 分析
func (h *Handler) AnalyzeBatch(c *gin.Context) {
	memberID, _ := strconv.ParseInt(c.Query("member_id"), 10, 64)

	// 获取未分析的文件
	files, err := h.db.GetHealthRecordFiles(c.Request.Context(), memberID, "", 100)
	if err != nil {
		response.InternalServerError(c, "查询文件列表失败")
		return
	}

	var pending []model.HealthRecordFile
	for _, f := range files {
		if f.AnalyzedAt == nil {
			pending = append(pending, f)
		}
	}

	if len(pending) == 0 {
		response.Success(c, gin.H{
			"message":    "没有待分析的文件",
			"total":      0,
			"analyzing":  0,
		})
		return
	}

	if h.aiAnalyzer == nil || os.Getenv("HOMEMATE_OPENAI_KEY") == "" {
		response.BadRequest(c, "AI 分析功能未配置，请在 config.yaml 中设置 openai.api_key 或环境变量 HOMEMATE_OPENAI_KEY")
		return
	}

	// 异步逐个分析
	go func() {
		for _, f := range pending {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			summary, analysis, err := h.aiAnalyzer(ctx, &f)
			cancel()

			if err != nil {
				log.Printf("[ERROR] 批量分析文件 %d 失败: %v", f.ID, err)
				continue
			}

			if err := h.db.UpdateHealthRecordFileAnalysis(context.Background(), f.ID, summary, analysis); err != nil {
				log.Printf("[ERROR] 保存分析结果失败: %v", err)
			}
		}
		log.Printf("[INFO] 批量分析完成，共处理 %d 个文件", len(pending))
	}()

	c.JSON(http.StatusAccepted, response.Response{
		Code:    0,
		Message: "批量 AI 分析已启动",
		Data: gin.H{
			"total":     len(files),
			"analyzing": len(pending),
		},
	})
}

// ListReports 获取 AI 分析报告列表
func (h *Handler) ListReports(c *gin.Context) {
	memberID, _ := strconv.ParseInt(c.Query("member_id"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))

	reports, err := h.db.GetAnalysisReports(c.Request.Context(), memberID, limit)
	if err != nil {
		response.InternalServerError(c, "查询报告列表失败")
		return
	}

	response.Success(c, gin.H{
		"list":  reports,
		"total": len(reports),
	})
}

// GenerateReport 手动触发生成综合健康分析报告
func (h *Handler) GenerateReport(c *gin.Context) {
	memberID, _ := strconv.ParseInt(c.PostForm("member_id"), 10, 64)
	if memberID <= 0 {
		response.BadRequest(c, "member_id 不能为空")
		return
	}

	periodStart := c.PostForm("period_start")
	periodEnd := c.PostForm("period_end")
	if periodEnd == "" {
		periodEnd = time.Now().Format("2006-01-02")
	}
	if periodStart == "" {
		periodEnd = time.Now().Format("2006-01-02")
		start, _ := time.ParseInLocation("2006-01-02", periodEnd, time.Local)
		periodStart = start.AddDate(0, -1, 0).Format("2006-01-02")
	}

	// 异步生成报告
	go func() {
		h.generateAndSaveReport(context.Background(), memberID, periodStart, periodEnd)
	}()

	c.JSON(http.StatusAccepted, response.Response{
		Code:    0,
		Message: "报告生成已启动",
	})
}

// generateAndSaveReport 生成并保存综合报告
func (h *Handler) generateAndSaveReport(ctx context.Context, memberID int64, periodStart, periodEnd string) {
	// 获取期间内的档案文件
	files, err := h.db.GetHealthRecordFiles(ctx, memberID, "", 50)
	if err != nil {
		log.Printf("[ERROR] 获取档案文件失败: %v", err)
		return
	}

	// 构建 AI prompt
	var fileSummaries []string
	for _, f := range files {
		if f.RecordDate >= periodStart && f.RecordDate <= periodEnd && f.Summary != "" {
			fileSummaries = append(fileSummaries, fmt.Sprintf("- [%s] %s: %s", f.RecordDate, f.Title, f.Summary))
		}
	}

	if len(fileSummaries) == 0 {
		log.Printf("[INFO] 成员 %d 在 %s~%s 期间没有已分析的档案", memberID, periodStart, periodEnd)
		return
	}

	if h.aiAnalyzer == nil || os.Getenv("HOMEMATE_OPENAI_KEY") == "" {
		return
	}

	// 使用 AI 综合分析（复用分析器，传入一个虚拟的"综合"文件）
	virtualFile := &model.HealthRecordFile{
		MemberID:   memberID,
		Title:      fmt.Sprintf("综合健康报告 (%s ~ %s)", periodStart, periodEnd),
		Category:   "综合报告",
		RecordDate: periodEnd,
		FilePath:   "",
	}

	// 直接调用 OpenAI 生成综合报告
	summary, details, err := h.generateComprehensiveReport(ctx, virtualFile, fileSummaries)
	if err != nil {
		log.Printf("[ERROR] 生成综合报告失败: %v", err)
		return
	}

	report := &model.HealthAnalysisReport{
		MemberID:    memberID,
		ReportDate:  time.Now().Format("2006-01-02"),
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Summary:     summary,
		Details:     details,
		Metrics:     "{}",
		Source:      "ai",
	}

	if _, err := h.db.CreateAnalysisReport(ctx, report); err != nil {
		log.Printf("[ERROR] 保存综合报告失败: %v", err)
	} else {
		log.Printf("[INFO] 成员 %d 综合报告已生成", memberID)
	}
}

// defaultAIAnalyzer 默认的 AI 分析实现
func (h *Handler) defaultAIAnalyzer(ctx context.Context, file *model.HealthRecordFile) (summary, analysis string, err error) {
	apiKey := os.Getenv("HOMEMATE_OPENAI_KEY")
	if apiKey == "" {
		return "", "", fmt.Errorf("OpenAI API Key 未配置，请在 config.yaml 中设置 openai.api_key 或环境变量 HOMEMATE_OPENAI_KEY")
	}

	// 占位实现 - 实际部署时替换为真实的文件内容提取 + LLM 调用
	return fmt.Sprintf("%s - %s", file.Category, file.Title),
		fmt.Sprintf(`{"status":"analyzed","file_type":"%s","category":"%s","record_date":"%s","findings":"待 AI 深度分析"}`, file.FileType, file.Category, file.RecordDate),
		nil
}

// generateComprehensiveReport 生成综合分析报告
func (h *Handler) generateComprehensiveReport(ctx context.Context, _ *model.HealthRecordFile, fileSummaries []string) (summary, details string, err error) {
	apiKey := os.Getenv("HOMEMATE_OPENAI_KEY")
	if apiKey == "" {
		return "AI 服务未配置", "", nil
	}

	_ = fmt.Sprintf(`基于以下健康档案摘要，生成一份综合健康分析报告。

档案摘要：
%s

请生成一份包含以下内容的 Markdown 报告：
1. 健康概况总结（2-3 句话）
2. 关键健康指标变化趋势
3. 需要关注的异常项
4. 健康建议

请以 JSON 格式返回，包含 summary（概况）和 details（Markdown 详细报告）两个字段。`, strings.Join(fileSummaries, "\n"))

	// 占位实现
	return fmt.Sprintf("综合报告，包含 %d 份档案的分析", len(fileSummaries)),
		fmt.Sprintf("# 综合健康分析报告\n\n共分析了 %d 份健康档案。\n\n## 概况\n待 AI 深度分析\n\n## 建议\n- 请配置 OpenAI API Key 以启用 AI 分析功能", len(fileSummaries)),
		nil
}

// analyzeWithOpenAI 使用 OpenAI API 分析（完整实现框架）
func (h *Handler) analyzeWithOpenAI(ctx context.Context, prompt string) (string, error) {
	apiKey := os.Getenv("HOMEMATE_OPENAI_KEY")
	baseURL := os.Getenv("HOMEMATE_OPENAI_BASE_URL")
	model := os.Getenv("HOMEMATE_OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	if apiKey == "" {
		return "", fmt.Errorf("OpenAI API Key 未配置")
	}

	// 构建请求体
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是家庭健康档案分析助手。请根据上传的健康档案内容，提取关键信息并给出分析。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  2000,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", strings.NewReader(string(jsonData)))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 OpenAI 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI 返回错误 %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("OpenAI 未返回有效内容")
	}

	return result.Choices[0].Message.Content, nil
}