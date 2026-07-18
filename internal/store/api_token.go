package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/homemate/server/internal/model"
)

// HashToken 计算 token 的 SHA256 哈希
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// CreateAPIToken 创建 API 令牌
func (db *DB) CreateAPIToken(ctx context.Context, name, tokenPlain string, scopes []string, expiresAt *time.Time) (int64, error) {
	scopesJSON, _ := json.Marshal(scopes)
	hash := HashToken(tokenPlain)
	var expiresVal interface{}
	if expiresAt != nil {
		expiresVal = expiresAt
	}
	res, err := db.conn.ExecContext(ctx,
		`INSERT INTO api_tokens (name, token_hash, scopes, is_active, created_at, expires_at)
		 VALUES (?, ?, ?, 1, datetime('now'), ?)`,
		name, hash, string(scopesJSON), expiresVal)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetAPITokenByHash 通过哈希查询令牌
func (db *DB) GetAPITokenByHash(ctx context.Context, hash string) (*model.APIToken, error) {
	row := db.conn.QueryRowContext(ctx,
		"SELECT id, name, token_hash, scopes, is_active, created_at, last_used_at, expires_at FROM api_tokens WHERE token_hash=? AND is_active=1",
		hash)
	return scanAPIToken(row)
}

// ListAPITokens 列出所有令牌
func (db *DB) ListAPITokens(ctx context.Context) ([]model.APITokenView, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, name, token_hash, scopes, is_active, created_at, last_used_at, expires_at FROM api_tokens ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.APITokenView
	for rows.Next() {
		var t model.APIToken
		var scopesJSON string
		var lastUsed sql.NullTime
		var expires sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.TokenHash, &scopesJSON, &t.IsActive, &t.CreatedAt, &lastUsed, &expires); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(scopesJSON), &t.Scopes)
		if lastUsed.Valid {
			t.LastUsedAt = &lastUsed.Time
		}
		if expires.Valid {
			t.ExpiresAt = &expires.Time
		}
		result = append(result, *t.ToView())
	}
	return result, rows.Err()
}

// UpdateAPITokenLastUsed 更新最后使用时间
func (db *DB) UpdateAPITokenLastUsed(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx,
		"UPDATE api_tokens SET last_used_at=datetime('now') WHERE id=?", id)
	return err
}

// DeactivateAPIToken 停用令牌
func (db *DB) DeactivateAPIToken(ctx context.Context, id int64) error {
	res, err := db.conn.ExecContext(ctx, "UPDATE api_tokens SET is_active=0 WHERE id=?", id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("令牌不存在")
	}
	return nil
}

// DeleteAPIToken 物理删除令牌
func (db *DB) DeleteAPIToken(ctx context.Context, id int64) error {
	res, err := db.conn.ExecContext(ctx, "DELETE FROM api_tokens WHERE id=?", id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("令牌不存在")
	}
	return nil
}

// scanAPIToken 扫描单行
func scanAPIToken(row *sql.Row) (*model.APIToken, error) {
	var t model.APIToken
	var scopesJSON string
	var lastUsed sql.NullTime
	var expires sql.NullTime
	err := row.Scan(&t.ID, &t.Name, &t.TokenHash, &scopesJSON, &t.IsActive, &t.CreatedAt, &lastUsed, &expires)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopesJSON), &t.Scopes)
	if lastUsed.Valid {
		t.LastUsedAt = &lastUsed.Time
	}
	if expires.Valid {
		t.ExpiresAt = &expires.Time
	}
	return &t, nil
}
