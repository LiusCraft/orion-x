package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore 使用 SQLite 持久化记忆。
type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("memory db path is empty")
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	uri := path
	if !strings.HasPrefix(path, "file:") {
		uri = "file:" + path + "?cache=shared&mode=rwc"
	}

	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) initSchema() error {
	stmts := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		`CREATE TABLE IF NOT EXISTS turns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT,
			session_id TEXT,
			device_id TEXT,
			turn_id INTEGER,
			user_text TEXT,
			assistant_text TEXT,
			started_at INTEGER,
			ended_at INTEGER,
			aborted INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			content TEXT NOT NULL,
			type TEXT,
			importance INTEGER,
			created_at INTEGER,
			expires_at INTEGER
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memories_user_created ON memories(user_id, created_at);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			content,
			user_id UNINDEXED,
			type UNINDEXED,
			memory_id UNINDEXED
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) SaveTurn(turn Turn) error {
	_, err := s.db.Exec(
		`INSERT INTO turns (user_id, session_id, device_id, turn_id, user_text, assistant_text, started_at, ended_at, aborted)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		turn.UserID,
		turn.SessionID,
		turn.DeviceID,
		turn.TurnID,
		turn.UserText,
		turn.AssistantText,
		toUnix(turn.StartedAt),
		toUnix(turn.EndedAt),
		boolToInt(turn.Aborted),
	)
	return err
}

func (s *SQLiteStore) SaveItems(items []MemoryItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	insertMem := `INSERT INTO memories (user_id, content, type, importance, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?);`
	insertFTS := `INSERT INTO memories_fts (content, user_id, type, memory_id)
		VALUES (?, ?, ?, ?);`

	for _, item := range items {
		res, execErr := tx.Exec(insertMem,
			item.UserID,
			item.Content,
			item.Type,
			item.Importance,
			toUnix(item.CreatedAt),
			toUnixPtr(item.ExpiresAt),
		)
		if execErr != nil {
			err = execErr
			return err
		}
		id, execErr := res.LastInsertId()
		if execErr != nil {
			err = execErr
			return err
		}
		_, execErr = tx.Exec(insertFTS, item.Content, item.UserID, item.Type, id)
		if execErr != nil {
			err = execErr
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) Query(userID, query string, limit int, minScore float64) ([]MemoryItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 6
	}

	items, err := s.queryFTS(userID, query, limit, minScore)
	if err != nil {
		return s.queryRecent(userID, limit)
	}
	if len(items) == 0 {
		return s.queryRecent(userID, limit)
	}
	return items, nil
}

func (s *SQLiteStore) queryFTS(userID, query string, limit int, minScore float64) ([]MemoryItem, error) {
	now := time.Now().Unix()
	rows, err := s.db.Query(
		`SELECT m.id, m.content, m.type, m.importance, m.created_at, m.expires_at, -bm25(memories_fts) AS score
		FROM memories_fts
		JOIN memories m ON m.id = memories_fts.memory_id
		WHERE memories_fts MATCH ?
			AND m.user_id = ?
			AND (m.expires_at IS NULL OR m.expires_at > ?)
		ORDER BY score DESC, m.importance DESC, m.created_at DESC
		LIMIT ?;`,
		query, userID, now, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]MemoryItem, 0, limit)
	for rows.Next() {
		var item MemoryItem
		var createdAt, expiresAt sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Content, &item.Type, &item.Importance, &createdAt, &expiresAt, &item.Score); err != nil {
			return nil, err
		}
		item.UserID = userID
		if createdAt.Valid {
			item.CreatedAt = time.Unix(createdAt.Int64, 0)
		}
		if expiresAt.Valid {
			t := time.Unix(expiresAt.Int64, 0)
			item.ExpiresAt = &t
		}
		if item.Score < minScore {
			continue
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLiteStore) queryRecent(userID string, limit int) ([]MemoryItem, error) {
	now := time.Now().Unix()
	rows, err := s.db.Query(
		`SELECT id, content, type, importance, created_at, expires_at, 0 AS score
		FROM memories
		WHERE user_id = ?
			AND (expires_at IS NULL OR expires_at > ?)
			AND (importance IS NULL OR importance >= 3)
		ORDER BY created_at DESC
		LIMIT ?;`,
		userID, now, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]MemoryItem, 0, limit)
	for rows.Next() {
		var item MemoryItem
		var createdAt, expiresAt sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Content, &item.Type, &item.Importance, &createdAt, &expiresAt, &item.Score); err != nil {
			return nil, err
		}
		item.UserID = userID
		if createdAt.Valid {
			item.CreatedAt = time.Unix(createdAt.Int64, 0)
		}
		if expiresAt.Valid {
			t := time.Unix(expiresAt.Int64, 0)
			item.ExpiresAt = &t
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLiteStore) Purge(now time.Time, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	threshold := now.Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.Exec(`DELETE FROM memories_fts WHERE memory_id IN (SELECT id FROM memories WHERE expires_at IS NOT NULL AND expires_at <= ?);`, threshold)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM memories WHERE expires_at IS NOT NULL AND expires_at <= ?;`, threshold)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func toUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func toUnixPtr(t *time.Time) interface{} {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.Unix()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
