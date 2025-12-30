package quotes

import (
	"bufio"
	"crypto/rand"
	"errors"
	"math/big"
	"os"
	"strings"
	"sync"
)

// Store provides access to wisdom quotes.
type Store interface {
	// Random returns a random quote from the store.
	Random() string

	// Count returns the number of quotes in the store.
	Count() int
}

// Default quote when store is empty.
const DefaultQuote = "The only true wisdom is in knowing you know nothing. - Socrates"

var (
	// ErrEmptyFile indicates the quotes file is empty.
	ErrEmptyFile = errors.New("quotes file is empty")
)

// memoryStore is an in-memory implementation of Store.
type memoryStore struct {
	mu     sync.RWMutex
	quotes []string
}

// NewMemoryStore creates a new in-memory quote store.
func NewMemoryStore(quotes []string) Store {
	// Make a copy to prevent external modification
	quoteCopy := make([]string, len(quotes))
	copy(quoteCopy, quotes)

	return &memoryStore{
		quotes: quoteCopy,
	}
}

// Random returns a random quote from the store.
// Returns DefaultQuote if the store is empty.
func (s *memoryStore) Random() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.quotes) == 0 {
		return DefaultQuote
	}

	// Use crypto/rand for unbiased selection
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(s.quotes))))
	if err != nil {
		// Fallback to first quote on random failure (extremely rare)
		return s.quotes[0]
	}

	return s.quotes[n.Int64()]
}

// Count returns the number of quotes in the store.
func (s *memoryStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.quotes)
}

// LoadFromFile reads quotes from a file, one quote per line.
// Empty lines and lines starting with # are ignored.
func LoadFromFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var quotes []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		quotes = append(quotes, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return quotes, nil
}

// MustLoadFromFile loads quotes from a file or panics.
// Use this for required configuration files at startup.
func MustLoadFromFile(path string) []string {
	quotes, err := LoadFromFile(path)
	if err != nil {
		panic("failed to load quotes: " + err.Error())
	}
	return quotes
}

// DefaultQuotes returns a set of built-in wisdom quotes.
func DefaultQuotes() []string {
	return []string{
		"The only true wisdom is in knowing you know nothing. - Socrates",
		"In the middle of difficulty lies opportunity. - Albert Einstein",
		"The unexamined life is not worth living. - Socrates",
		"Wisdom begins in wonder. - Socrates",
		"The only way to do great work is to love what you do. - Steve Jobs",
		"It is not that I'm so smart. But I stay with the questions much longer. - Albert Einstein",
		"The measure of intelligence is the ability to change. - Albert Einstein",
		"Know thyself. - Ancient Greek Aphorism",
		"To be yourself in a world that is constantly trying to make you something else is the greatest accomplishment. - Ralph Waldo Emerson",
		"The mind is everything. What you think you become. - Buddha",
		"Happiness is not something ready made. It comes from your own actions. - Dalai Lama",
		"The best time to plant a tree was 20 years ago. The second best time is now. - Chinese Proverb",
		"An unexamined life is not worth living. - Socrates",
		"He who knows others is wise; he who knows himself is enlightened. - Lao Tzu",
		"The journey of a thousand miles begins with a single step. - Lao Tzu",
		"What we think, we become. - Buddha",
		"The only thing I know is that I know nothing. - Socrates",
		"Life is really simple, but we insist on making it complicated. - Confucius",
		"Do not dwell in the past, do not dream of the future, concentrate the mind on the present moment. - Buddha",
		"The way to get started is to quit talking and begin doing. - Walt Disney",
	}
}
