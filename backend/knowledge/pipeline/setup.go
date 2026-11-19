package pipeline

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"

	"review-dock/backend/knowledge/db"
	"review-dock/backend/models"
)

// NewLogger returns a structured JSON logger writing to
// <dataDir>/logs/knowledge.log (rotated), plus extra writers (e.g. stderr).
func NewLogger(dataDir string, extra io.Writer) *slog.Logger {
	logDir := filepath.Join(dataDir, "logs")
	_ = os.MkdirAll(logDir, 0o755)
	var w io.Writer = &lumberjack.Logger{
		Filename: filepath.Join(logDir, "knowledge.log"), MaxSize: 20, MaxBackups: 5, MaxAge: 30,
	}
	if extra != nil {
		w = io.MultiWriter(w, extra)
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// DBPath is the knowledge database location inside the app data dir.
func DBPath(dataDir string) string { return filepath.Join(dataDir, "knowledge.db") }

// Open opens the database and returns a ready pipeline.
func Open(dataDir string, log *slog.Logger) (*Pipeline, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	d, err := db.Open(DBPath(dataDir))
	if err != nil {
		return nil, err
	}
	return &Pipeline{DB: d, DataDir: dataDir, Log: log}, nil
}

// RegisterTracked makes every repository tracked by the desktop app
// (repos.json) known to the knowledge DB. Existing settings are kept.
func (p *Pipeline) RegisterTracked(ctx context.Context, repos []models.Repository) (int, error) {
	added := 0
	for _, r := range repos {
		if r.Owner == "" || r.Name == "" {
			continue
		}
		if _, err := p.DB.GetRepositoryByName(ctx, r.Owner+"/"+r.Name); err == nil {
			continue
		}
		if _, err := p.DB.EnsureRepository(ctx, r.Owner, r.Name); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}
