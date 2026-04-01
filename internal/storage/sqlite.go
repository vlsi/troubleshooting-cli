package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vlsi/troubleshooting-cli/internal/domain"
	_ "modernc.org/sqlite"
)

// SQLiteStore implements app.Store using a local SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// DefaultDBPath returns the default database path under ~/.troubleshooting/sessions.db.
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".troubleshooting")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions.db"), nil
}

// NewSQLiteStore opens or creates a SQLite database at the given path.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate() error {
	ddl := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		data TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS findings (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		data TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS hypotheses (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		data TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS timeline (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		timestamp TEXT NOT NULL,
		data TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_findings_session ON findings(session_id);
	CREATE INDEX IF NOT EXISTS idx_hypotheses_session ON hypotheses(session_id);
	CREATE INDEX IF NOT EXISTS idx_timeline_session ON timeline(session_id, timestamp);
	`
	_, err := s.db.Exec(ddl)
	return err
}

func (s *SQLiteStore) CreateSession(sess domain.Session) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO sessions (id, data) VALUES (?, ?)", sess.ID, string(data))
	return err
}

func (s *SQLiteStore) GetSession(id string) (domain.Session, error) {
	var data string
	err := s.db.QueryRow("SELECT data FROM sessions WHERE id = ?", id).Scan(&data)
	if err != nil {
		return domain.Session{}, fmt.Errorf("session %s: %w", id, err)
	}
	var sess domain.Session
	return sess, json.Unmarshal([]byte(data), &sess)
}

func (s *SQLiteStore) UpdateSession(sess domain.Session) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE sessions SET data = ? WHERE id = ?", string(data), sess.ID)
	return err
}

func (s *SQLiteStore) AddFinding(f domain.Finding) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO findings (id, session_id, data) VALUES (?, ?, ?)", f.ID, f.SessionID, string(data))
	return err
}

func (s *SQLiteStore) ListFindings(sessionID string) ([]domain.Finding, error) {
	rows, err := s.db.Query("SELECT data FROM findings WHERE session_id = ?", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Finding
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var f domain.Finding
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) AddHypothesis(h domain.Hypothesis) error {
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO hypotheses (id, session_id, data) VALUES (?, ?, ?)", h.ID, h.SessionID, string(data))
	return err
}

func (s *SQLiteStore) GetHypothesis(id string) (domain.Hypothesis, error) {
	var data string
	err := s.db.QueryRow("SELECT data FROM hypotheses WHERE id = ?", id).Scan(&data)
	if err != nil {
		return domain.Hypothesis{}, fmt.Errorf("hypothesis %s: %w", id, err)
	}
	var h domain.Hypothesis
	return h, json.Unmarshal([]byte(data), &h)
}

func (s *SQLiteStore) UpdateHypothesis(h domain.Hypothesis) error {
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE hypotheses SET data = ? WHERE id = ?", string(data), h.ID)
	return err
}

func (s *SQLiteStore) ListHypotheses(sessionID string) ([]domain.Hypothesis, error) {
	rows, err := s.db.Query("SELECT data FROM hypotheses WHERE session_id = ?", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Hypothesis
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var h domain.Hypothesis
		if err := json.Unmarshal([]byte(data), &h); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) AddTimelineEvent(e domain.TimelineEvent) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO timeline (id, session_id, timestamp, data) VALUES (?, ?, ?, ?)",
		e.ID, e.SessionID, e.Timestamp.Format(time.RFC3339Nano), string(data))
	return err
}

func (s *SQLiteStore) ListTimeline(sessionID string) ([]domain.TimelineEvent, error) {
	rows, err := s.db.Query("SELECT data FROM timeline WHERE session_id = ? ORDER BY timestamp ASC", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.TimelineEvent
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var e domain.TimelineEvent
		if err := json.Unmarshal([]byte(data), &e); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
