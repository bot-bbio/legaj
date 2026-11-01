// Package secrets keeps the Gemini API key in Windows Credential Manager
// and nowhere else (R4, D14): never in the DB, a file, a log or an error.
//
// The rest of the app uses Manager. Its Status method is the only view of
// the key that may reach the frontend: saved or not, plus a mask showing
// the last 4 characters.
package secrets

import (
	"errors"
	"strings"
	"sync"
	"unicode"

	"github.com/zalando/go-keyring"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/logx"
)

// ErrNotFound means no key is saved.
var ErrNotFound = errors.New("no API key saved")

// Store is where the key lives.
type Store interface {
	Get() (string, error) // ErrNotFound when none is saved
	Set(key string) error
	Delete() error // succeeds when none is saved
}

// Credential Manager entry: shown to users as "LeGaJ:gemini-api-key".
const (
	keyringService = "LeGaJ"
	keyringUser    = "gemini-api-key"
)

// Keyring stores the key in Windows Credential Manager, encrypted with the
// user's login via DPAPI.
type Keyring struct {
	service, user string
}

// NewKeyring returns the app's Credential Manager store.
func NewKeyring() *Keyring {
	return NewKeyringFor(keyringService, keyringUser)
}

// NewKeyringFor targets a specific entry (tests use a throwaway one).
func NewKeyringFor(service, user string) *Keyring {
	return &Keyring{service: service, user: user}
}

// Get reads the key.
func (k *Keyring) Get() (string, error) {
	v, err := keyring.Get(k.service, k.user)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

// Set writes the key, replacing any previous one.
func (k *Keyring) Set(key string) error {
	return keyring.Set(k.service, k.user, key)
}

// Delete removes the key. Removing a missing key succeeds.
func (k *Keyring) Delete() error {
	if err := keyring.Delete(k.service, k.user); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// Memory is an in-process Store for tests.
type Memory struct {
	mu  sync.Mutex
	key string
	set bool
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{}
}

// Get returns the stored key or ErrNotFound.
func (m *Memory) Get() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.set {
		return "", ErrNotFound
	}
	return m.key, nil
}

// Set stores the key.
func (m *Memory) Set(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.key, m.set = key, true
	return nil
}

// Delete forgets the key.
func (m *Memory) Delete() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.key, m.set = "", false
	return nil
}

// Mask returns "••••" plus the last 4 characters, or "" for no key.
func Mask(key string) string {
	if key == "" {
		return ""
	}
	r := []rune(key)
	if len(r) <= 4 {
		return "••••"
	}
	return "••••" + string(r[len(r)-4:])
}

// Status is what Settings may show about the key.
type Status struct {
	Saved  bool   `json:"saved"`
	Masked string `json:"masked"`
}

// Manager is the app's single entry point to the key.
type Manager struct {
	store    Store
	redactor *logx.Redactor
}

// NewManager wires a store to the log redactor.
func NewManager(s Store, r *logx.Redactor) *Manager {
	return &Manager{store: s, redactor: r}
}

// Load returns the key for an AI call and registers it with the log
// redactor. A missing key is an apperr with CodeAIKeyMissing.
func (m *Manager) Load() (string, error) {
	key, err := m.store.Get()
	if errors.Is(err, ErrNotFound) {
		return "", apperr.New(apperr.CodeAIKeyMissing, "Add your Gemini API key in Settings first.")
	}
	if err != nil {
		return "", apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your API key from Windows Credential Manager.")
	}
	m.redactor.Register(key)
	return key, nil
}

// Save validates and stores a key the user pasted (surrounding whitespace
// is trimmed). Error messages never echo the input.
func (m *Manager) Save(key string) error {
	key = strings.TrimSpace(key)
	if err := validate(key); err != nil {
		return err
	}
	m.redactor.Register(key)
	if err := m.store.Set(key); err != nil {
		return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save your API key to Windows Credential Manager.")
	}
	return nil
}

// Remove deletes the key.
func (m *Manager) Remove() error {
	if err := m.store.Delete(); err != nil {
		return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't remove your API key from Windows Credential Manager.")
	}
	return nil
}

// Status reports whether a key is saved, masked. It never returns the key.
func (m *Manager) Status() (Status, error) {
	key, err := m.store.Get()
	if errors.Is(err, ErrNotFound) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read your API key from Windows Credential Manager.")
	}
	m.redactor.Register(key)
	return Status{Saved: true, Masked: Mask(key)}, nil
}

// Key length bounds: generous, because the provider's format may change.
const (
	minKeyLen = 16
	maxKeyLen = 512
)

func validate(key string) error {
	invalid := apperr.New(apperr.CodeInvalid, "That doesn't look like an API key. Copy it again from Google AI Studio.")
	if len(key) < minKeyLen || len(key) > maxKeyLen {
		return invalid
	}
	for _, r := range key {
		if r > unicode.MaxASCII || unicode.IsSpace(r) || unicode.IsControl(r) {
			return invalid
		}
	}
	return nil
}
