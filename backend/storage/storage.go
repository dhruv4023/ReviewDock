package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
	"my-github-pr/backend/models"
)

type Service struct {
	mu       sync.RWMutex
	dataDir  string
	lockPath string
}

func NewService(appName string) (*Service, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("could not determine user config dir: %w", err)
	}

	dataDir := filepath.Join(userConfigDir, appName)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("could not create data dir: %w", err)
	}

	return &Service{
		dataDir:  dataDir,
		lockPath: filepath.Join(dataDir, "app.lock"),
	}, nil
}

func (s *Service) getLock() *flock.Flock {
	return flock.New(s.lockPath)
}

func (s *Service) ReadRepos() ([]models.Repository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := filepath.Join(s.dataDir, "repos.json")
	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []models.Repository{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var data struct {
		Repositories []models.Repository `json:"repositories"`
	}

	if err := json.NewDecoder(file).Decode(&data); err != nil {
		if errors.Is(err, io.EOF) {
			return []models.Repository{}, nil
		}
		return nil, err
	}

	return data.Repositories, nil
}

func (s *Service) WriteRepos(repos []models.Repository) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fl := s.getLock()
	if err := fl.Lock(); err != nil {
		return err
	}
	defer fl.Unlock()

	filePath := filepath.Join(s.dataDir, "repos.json")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	data := struct {
		Repositories []models.Repository `json:"repositories"`
	}{
		Repositories: repos,
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func (s *Service) ReadSettings() (*models.Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := filepath.Join(s.dataDir, "settings.json")
	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &models.Settings{
				ConcurrencyLimit:      3,
				DefaultRemotePriority: []string{"origin", "upstream", "odoo", "ent"},
				AmendCommitTimestamp:  true,
				ForcePushAfterRebase:  false,
				AutoRefreshInterval:   10,
				Theme:                 "dark",
			}, nil
		}
		return nil, err
	}
	defer file.Close()

	var settings models.Settings
	if err := json.NewDecoder(file).Decode(&settings); err != nil {
		return nil, err
	}

	return &settings, nil
}

func (s *Service) WriteSettings(settings *models.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fl := s.getLock()
	if err := fl.Lock(); err != nil {
		return err
	}
	defer fl.Unlock()

	filePath := filepath.Join(s.dataDir, "settings.json")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(settings)
}

func (s *Service) ReadSession() (*models.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := filepath.Join(s.dataDir, "sessions.json")
	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var session models.Session
	if err := json.NewDecoder(file).Decode(&session); err != nil {
		return nil, err
	}

	return &session, nil
}

func (s *Service) WriteSession(session *models.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fl := s.getLock()
	if err := fl.Lock(); err != nil {
		return err
	}
	defer fl.Unlock()

	filePath := filepath.Join(s.dataDir, "sessions.json")
	if session == nil {
		return os.Remove(filePath)
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(session)
}
