package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Configuration constants
const (
	version         = "0.2.1"
	defaultShell    = "/bin/bash"
	defaultTriesDir = "src/tries"
	configFileName  = "config"
	configDirName   = ".config/try"
)

type Config struct {
	Path  string `json:"path"`
	Shell string `json:"shell,omitempty"`
}

// sanitizePath validates and cleans a path to prevent path traversal attacks
func sanitizePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	// Clean the path to resolve . and .. elements
	cleaned := filepath.Clean(path)

	// Expand ~ to home directory
	if strings.HasPrefix(cleaned, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~: %w", err)
		}
		if cleaned == "~" {
			cleaned = home
		} else if strings.HasPrefix(cleaned, "~/") {
			cleaned = filepath.Join(home, cleaned[2:])
		}
	}

	// Make absolute if relative
	if !filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("cannot resolve path: %w", err)
		}
		cleaned = abs
	}

	// Final clean to normalize
	cleaned = filepath.Clean(cleaned)

	return cleaned, nil
}

// validateShell checks if a shell executable exists and is valid
func validateShell(shell string) error {
	if shell == "" {
		return nil
	}

	// Check if shell is an absolute path
	if !filepath.IsAbs(shell) {
		return fmt.Errorf("shell must be an absolute path: %s", shell)
	}

	// Check if shell exists and is executable
	if _, err := exec.LookPath(shell); err != nil {
		return fmt.Errorf("shell not found or not executable: %s", shell)
	}

	return nil
}

// validateConfig validates and sanitizes config values
func (c *Config) Validate() error {
	if c.Path != "" {
		sanitized, err := sanitizePath(c.Path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		c.Path = sanitized
	}

	if c.Shell != "" {
		if err := validateShell(c.Shell); err != nil {
			return fmt.Errorf("invalid shell: %w", err)
		}
	}

	return nil
}

type tryEntry struct {
	Name     string
	Basename string
	Path     string
	IsNew    bool
	CTime    time.Time
	MTime    time.Time
	Score    float64
}

type model struct {
	tries         []tryEntry
	filteredTries []tryEntry
	cursor        int
	scrollOffset  int
	searchTerm    string
	selected      *selection
	basePath      string
	config        *Config
	width         int
	height        int
	quitting      bool
	inputMode     bool
	newName       string
	confirmDelete       bool
	deleteTarget        *tryEntry
	deleteConfirmBuffer string
	// Rename mode
	renameMode   bool
	renameTarget *tryEntry
	renameName   string
	// Text editing cursor position (shared across input modes)
	cursorPos int
}

type selection struct {
	Type     string
	Path     string
	CloneURL string // For clone operations
	OldPath  string // For rename operations
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("220")).
			MarginBottom(1)

	searchStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86"))

	searchInputStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

	matchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

	dateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("237"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	createNewStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82"))

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	dangerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
)

func getConfigPath() string {
	// Always use ~/.config/try for consistency across platforms
	// This avoids macOS Application Support restrictions and symlink issues
	home, err := os.UserHomeDir()
	if err != nil {
		// If we can't find home, return empty string
		// This will cause config operations to fail gracefully
		return ""
	}
	return filepath.Join(home, ".config", "try", configFileName)
}

// getLegacyConfigPaths returns old config locations for migration
func getLegacyConfigPaths() []string {
	_, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	newPath := getConfigPath()
	pathMap := make(map[string]bool)

	// macOS Application Support path from early v0.2.0
	configHome, err := os.UserConfigDir()
	if err == nil {
		appSupportPath := filepath.Join(configHome, "try", configFileName)
		if appSupportPath != newPath {
			pathMap[appSupportPath] = true
		}
	}

	// Convert map to slice (deduplicated)
	var paths []string
	for path := range pathMap {
		paths = append(paths, path)
	}

	return paths
}

// tryMigration attempts to migrate config from legacy locations
func tryMigration(configPath string) (*Config, bool, error) {
	for _, legacyPath := range getLegacyConfigPaths() {
		legacyData, legacyErr := os.ReadFile(legacyPath)
		if legacyErr != nil {
			continue
		}

		fmt.Fprintf(os.Stderr, "Note: Migrating config from %s to %s\n", legacyPath, configPath)

		// Parse legacy config
		var config Config
		if err := json.Unmarshal(legacyData, &config); err != nil {
			// Might be old plain text format
			path := strings.TrimSpace(string(legacyData))
			if path != "" {
				fmt.Fprintf(os.Stderr, "Note: Converting from plain text to JSON format\n")
				config = Config{Path: path}
			}
		}

		// Save to new location
		if err := saveConfig(&config); err != nil {
			return nil, false, fmt.Errorf("failed to save migrated config: %w", err)
		}

		// Successfully migrated, create backup and remove old config
		backupPath := legacyPath + ".bak"
		if err := os.Rename(legacyPath, backupPath); err != nil {
			// If rename fails, just try to remove
			os.Remove(legacyPath)
		} else {
			fmt.Fprintf(os.Stderr, "Note: Legacy config backed up to %s\n", backupPath)
		}

		return &config, true, nil
	}

	return nil, false, nil
}

func loadConfig() (*Config, error) {
	configPath := getConfigPath()
	if configPath == "" {
		// Cannot determine config path, use empty config
		fmt.Fprintf(os.Stderr, "Warning: cannot determine home directory, using ephemeral config\n")
		return &Config{}, nil
	}
	data, err := os.ReadFile(configPath)

	if err != nil {
		if os.IsNotExist(err) {
			// Try legacy locations for migration
			config, migrated, migErr := tryMigration(configPath)
			if migErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed during config migration check: %v\n", migErr)
			}
			if migrated {
				return config, nil
			}

			// No config found anywhere
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Try to parse as JSON first
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		// Might be old format (plain text path)
		path := strings.TrimSpace(string(data))
		if path != "" {
			fmt.Fprintf(os.Stderr, "Note: Migrating config from old format to new JSON format\n")
			return &Config{Path: path}, nil
		}
		return &Config{}, nil
	}

	return &config, nil
}

func saveConfig(config *Config) error {
	configPath := getConfigPath()
	if configPath == "" {
		return fmt.Errorf("cannot save config: home directory not found")
	}
	configDir := filepath.Dir(configPath)

	// Create config directory if it doesn't exist with restrictive permissions
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// getResolvedConfig loads config and applies environment variable overrides
func getResolvedConfig() (*Config, error) {
	// Always load config first
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	// Apply environment variable overrides
	if tryPath := os.Getenv("TRY_PATH"); tryPath != "" {
		config.Path = tryPath
	}

	if tryShell := os.Getenv("TRY_SHELL"); tryShell != "" {
		config.Shell = tryShell
	}

	// Validate and sanitize the final config
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func getDefaultPath(config *Config) string {
	if config != nil && config.Path != "" {
		return config.Path
	}

	// No default - will need to prompt
	return ""
}

func getShell(config *Config) string {
	// Config has already been resolved with environment variable overrides
	if config != nil && config.Shell != "" {
		return config.Shell
	}

	// Fall back to SHELL environment variable
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}

	// Final fallback
	return defaultShell
}

func promptForPath() string {
	home, _ := os.UserHomeDir()
	defaultPath := filepath.Join(home, defaultTriesDir)

	fmt.Println(titleStyle.Render("🎉 Welcome to Try!"))
	fmt.Println()
	fmt.Println("Try needs a directory to store your experiments.")
	fmt.Println("This will be created if it doesn't exist.")
	fmt.Println()
	fmt.Printf("%s [%s]: ",
		promptStyle.Render("Where should experiments be stored?"),
		dimStyle.Render(defaultPath))

	// Read the full line of input (allows spaces in paths)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError reading input: %v\n", err)
		os.Exit(1)
	}
	input = strings.TrimSpace(input)

	// Use default if empty
	if input == "" {
		input = defaultPath
	}

	// Sanitize the path
	absPath, err := sanitizePath(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid path: %v\n", err)
		os.Exit(1)
	}

	config := &Config{Path: absPath}

	// Now prompt for shell configuration
	fmt.Println()
	fmt.Println(promptStyle.Render("Shell Configuration (optional)"))
	currentShell := os.Getenv("SHELL")
	if currentShell == "" {
		currentShell = defaultShell
	}
	fmt.Printf("Current SHELL: %s\n", dimStyle.Render(currentShell))
	fmt.Print("Override shell (press Enter to use $SHELL): ")

	shellInput, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError reading input: %v\n", err)
		// Don't exit, just use default
	} else {
		shellInput = strings.TrimSpace(shellInput)
		if shellInput != "" {
			// Find the shell executable
			shellPath, err := exec.LookPath(shellInput)
			if err != nil {
				fmt.Printf("⚠️  Shell '%s' not found, using $SHELL\n", shellInput)
			} else {
				// Make absolute if it's not already
				if !filepath.IsAbs(shellPath) {
					shellPath, err = filepath.Abs(shellPath)
					if err != nil {
						fmt.Printf("⚠️  Cannot resolve shell path: %v\n", err)
						shellPath = ""
					}
				}
				if shellPath != "" {
					config.Shell = shellPath
					fmt.Printf("✅ Shell set to: %s\n", createNewStyle.Render(shellPath))
				}
			}
		}
	}

	// Store config
	if err := saveConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	// Show success message
	fmt.Println()
	fmt.Printf("✅ Experiments will be stored in: %s\n", createNewStyle.Render(absPath))
	if config.Shell != "" {
		fmt.Printf("✅ Shell override: %s\n", createNewStyle.Render(config.Shell))
	}
	fmt.Printf("%s\n", dimStyle.Render(fmt.Sprintf("(You can change these settings by editing %s)", getConfigPath())))
	fmt.Println()

	// Wait for user to acknowledge
	fmt.Print(helpStyle.Render("Press Enter to continue..."))
	bufio.NewReader(os.Stdin).ReadString('\n')

	return absPath
}

func initialModel(searchTerm string, config *Config) model {
	basePath := getDefaultPath(config)

	// If no path configured, prompt for it
	if basePath == "" {
		basePath = promptForPath()
		// Reload config after prompting
		var err error
		config, err = getResolvedConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to reload config after setting path: %v\n", err)
			os.Exit(1)
		}
	}

	// Ensure base path exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", basePath, err)
	}

	m := model{
		searchTerm: strings.ReplaceAll(searchTerm, " ", "-"),
		basePath:   basePath,
		config:     config,
		width:      80,
		height:     24,
	}

	m.loadTries()
	m.filterTries()
	return m
}

func (m *model) loadTries() {
	m.tries = []tryEntry{}

	entries, err := os.ReadDir(m.basePath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		path := filepath.Join(m.basePath, entry.Name())
		stat, _ := os.Stat(path)

		m.tries = append(m.tries, tryEntry{
			Name:     entry.Name(),
			Basename: entry.Name(),
			Path:     path,
			IsNew:    false,
			CTime:    info.ModTime(), // Go doesn't have creation time on all platforms
			MTime:    stat.ModTime(),
		})
	}
}

func (m *model) filterTries() {
	m.filteredTries = []tryEntry{}

	for _, try := range m.tries {
		score := m.calculateScore(try)
		try.Score = score

		if m.searchTerm == "" || score > 0 {
			m.filteredTries = append(m.filteredTries, try)
		}
	}

	// Sort by score descending
	sort.Slice(m.filteredTries, func(i, j int) bool {
		return m.filteredTries[i].Score > m.filteredTries[j].Score
	})
}

func (m *model) calculateScore(try tryEntry) float64 {
	score := 0.0

	// Bonus for date-prefixed directories
	if strings.HasPrefix(try.Basename, "20") && len(try.Basename) > 10 {
		if try.Basename[4] == '-' && try.Basename[7] == '-' && try.Basename[10] == '-' {
			score += 2.0
		}
	}

	// Search query matching
	if m.searchTerm != "" {
		textLower := strings.ToLower(try.Basename)
		queryLower := strings.ToLower(m.searchTerm)
		queryChars := []rune(queryLower)

		lastPos := -1
		queryIdx := 0

		for pos, char := range textLower {
			if queryIdx >= len(queryChars) {
				break
			}
			if char != queryChars[queryIdx] {
				continue
			}

			// Base point + word boundary bonus
			score += 1.0
			if pos == 0 || (pos > 0 && !isAlphaNum(rune(textLower[pos-1]))) {
				score += 1.0
			}

			// Proximity bonus
			if lastPos >= 0 {
				gap := pos - lastPos - 1
				score += 1.0 / math.Sqrt(float64(gap+1))
			}

			lastPos = pos
			queryIdx++
		}

		// Return 0 if not all query chars matched
		if queryIdx < len(queryChars) {
			return 0.0
		}

		// Density bonus
		if lastPos >= 0 {
			score *= float64(len(queryChars)) / float64(lastPos+1)
		}

		// Length penalty
		score *= 10.0 / (float64(len(try.Basename)) + 10.0)
	}

	// Time-based scoring
	now := time.Now()

	// Creation time bonus
	daysOld := now.Sub(try.CTime).Hours() / 24
	score += 2.0 / math.Sqrt(daysOld+1)

	// Access time bonus
	hoursAccess := now.Sub(try.MTime).Hours()
	score += 3.0 / math.Sqrt(hoursAccess+1)

	return score
}

func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// Text editing helper functions for cursor-aware editing

// insertCharAt inserts a character at the cursor position and returns the new string and cursor
func insertCharAt(text string, pos int, char rune) (string, int) {
	runes := []rune(text)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	newRunes := make([]rune, len(runes)+1)
	copy(newRunes[:pos], runes[:pos])
	newRunes[pos] = char
	copy(newRunes[pos+1:], runes[pos:])
	return string(newRunes), pos + 1
}

// insertStringAt inserts a string at the cursor position and returns the new string and cursor
func insertStringAt(text string, pos int, insert string) (string, int) {
	runes := []rune(text)
	insertRunes := []rune(insert)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	newRunes := make([]rune, len(runes)+len(insertRunes))
	copy(newRunes[:pos], runes[:pos])
	copy(newRunes[pos:], insertRunes)
	copy(newRunes[pos+len(insertRunes):], runes[pos:])
	return string(newRunes), pos + len(insertRunes)
}

// deleteCharBackward deletes the character before the cursor and returns the new string and cursor
func deleteCharBackward(text string, pos int) (string, int) {
	runes := []rune(text)
	if pos <= 0 || pos > len(runes) {
		return text, pos
	}
	newRunes := make([]rune, len(runes)-1)
	copy(newRunes[:pos-1], runes[:pos-1])
	copy(newRunes[pos-1:], runes[pos:])
	return string(newRunes), pos - 1
}

// deleteToEnd deletes from cursor to end of line and returns the new string
func deleteToEnd(text string, pos int) string {
	runes := []rune(text)
	if pos < 0 {
		pos = 0
	}
	if pos >= len(runes) {
		return text
	}
	return string(runes[:pos])
}

// deleteWordBackward deletes the word before the cursor and returns the new string and cursor
func deleteWordBackward(text string, pos int) (string, int) {
	runes := []rune(text)
	if pos <= 0 || pos > len(runes) {
		return text, pos
	}

	// Skip any trailing spaces
	newPos := pos
	for newPos > 0 && runes[newPos-1] == ' ' {
		newPos--
	}

	// Delete until next space or beginning
	for newPos > 0 && runes[newPos-1] != ' ' {
		newPos--
	}

	newRunes := make([]rune, len(runes)-(pos-newPos))
	copy(newRunes[:newPos], runes[:newPos])
	copy(newRunes[newPos:], runes[pos:])
	return string(newRunes), newPos
}

// runeLen returns the number of runes in a string
func runeLen(s string) int {
	return len([]rune(s))
}

// renderTextWithCursor renders text with a block cursor at the given position
func renderTextWithCursor(text string, pos int, textStyle, cursorStyle lipgloss.Style) string {
	runes := []rune(text)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}

	var result strings.Builder
	for i, r := range runes {
		if i == pos {
			// Render cursor character with cursor style
			result.WriteString(cursorStyle.Render(string(r)))
		} else {
			result.WriteString(textStyle.Render(string(r)))
		}
	}
	// If cursor is at end, show a block cursor
	if pos == len(runes) {
		result.WriteString(cursorStyle.Render(" "))
	}
	return result.String()
}

// isValidSearchInput checks if the input string contains only valid characters for search
func isValidSearchInput(input string) bool {
	for _, char := range input {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '.' || char == ' ' ||
			char == ':' || char == '/' || char == '@') {
			return false
		}
	}
	return len(input) > 0
}

// Pre-compiled GitHub URL patterns
var githubPatterns = []struct {
	regex  *regexp.Regexp
	format string
}{
	{regexp.MustCompile(`^https?://github\.com/([\w-]+)/([\w\.-]+?)(?:\.git)?/?$`), "https://github.com/$1/$2.git"},
	{regexp.MustCompile(`^github\.com/([\w-]+)/([\w\.-]+?)(?:\.git)?/?$`), "https://github.com/$1/$2.git"},
	{regexp.MustCompile(`^git@github\.com:([\w-]+)/([\w\.-]+?)(?:\.git)?$`), "https://github.com/$1/$2.git"},
	{regexp.MustCompile(`^gh:([\w-]+)/([\w\.-]+?)$`), "https://github.com/$1/$2.git"},
}

// isGitHubURL checks if the text is a GitHub URL and returns normalized clone URL
func isGitHubURL(text string) (bool, string) {
	text = strings.TrimSpace(text)

	for _, p := range githubPatterns {
		if matches := p.regex.FindStringSubmatch(text); matches != nil {
			user := matches[1]
			repo := matches[2]
			return true, fmt.Sprintf("https://github.com/%s/%s.git", user, repo)
		}
	}

	return false, ""
}

// extractRepoName extracts the repository name from a GitHub URL
func extractRepoName(url string) string {
	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// Extract repo name from URL
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		repoName := parts[len(parts)-1]
		// Sanitize: remove any path traversal attempts and invalid chars
		repoName = filepath.Base(repoName) // This removes any ../ attempts
		repoName = strings.ReplaceAll(repoName, "..", "")
		repoName = strings.ReplaceAll(repoName, "/", "-")
		repoName = strings.ReplaceAll(repoName, "\\", "-")
		if repoName == "" || repoName == "." {
			return "repo"
		}
		return repoName
	}

	return "repo"
}

// cloneRepository clones a git repository to the specified path with timeout
func cloneRepository(url, targetPath string) error {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is not installed")
	}

	// Create the target directory
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Clone the repository with timeout
	cmd := exec.Command("git", "clone", "--depth", "1", url, targetPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	// Set a 2-minute timeout for clone operation
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			// If clone failed, remove the directory
			os.RemoveAll(targetPath)
			return fmt.Errorf("failed to clone repository: %v", err)
		}
		return nil
	case <-time.After(2 * time.Minute):
		cmd.Process.Kill()
		os.RemoveAll(targetPath)
		return fmt.Errorf("clone operation timed out after 2 minutes")
	}
}

// performClone handles the common clone operation logic
func performClone(cloneURL, basePath string) (string, error) {
	// Extract repo name and create dated folder name
	repoName := extractRepoName(cloneURL)
	datePrefix := time.Now().Format("2006-01-02")
	dirName := fmt.Sprintf("%s-%s", datePrefix, repoName)
	fullPath := filepath.Join(basePath, dirName)

	// Check if directory already exists
	if _, err := os.Stat(fullPath); err == nil {
		// Add a number suffix if it exists
		for i := 2; ; i++ {
			testPath := fmt.Sprintf("%s-%d", fullPath, i)
			if _, err := os.Stat(testPath); os.IsNotExist(err) {
				fullPath = testPath
				dirName = fmt.Sprintf("%s-%d", dirName, i)
				break
			}
		}
	}

	// Clone the repository
	fmt.Printf("📦 Cloning %s into %s...\n", cloneURL, dirName)
	if err := cloneRepository(cloneURL, fullPath); err != nil {
		return "", err
	}

	return fullPath, nil
}

// Git worktree support functions

// isGitRepository checks if the given path is inside a git repository
func isGitRepository(path string) (string, bool) {
	// Walk up the directory tree looking for .git
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}

	for {
		gitPath := filepath.Join(absPath, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			// .git can be a directory (normal repo) or a file (worktree)
			if info.IsDir() || info.Mode().IsRegular() {
				return absPath, true
			}
		}

		parent := filepath.Dir(absPath)
		if parent == absPath {
			// Reached root
			return "", false
		}
		absPath = parent
	}
}

// sanitizeBranchName converts branch name to filesystem-safe format
// Replaces / with - to match GT convention
func sanitizeBranchName(branch string) string {
	sanitized := strings.ReplaceAll(branch, "/", "-")
	// Remove any other potentially problematic characters
	sanitized = strings.ReplaceAll(sanitized, "\\", "-")
	sanitized = strings.ReplaceAll(sanitized, "..", "-")
	return sanitized
}

// createWorktree creates a git worktree at the target path
func createWorktree(repoPath, targetPath, branch string) error {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is not installed")
	}

	// Create worktree with detached HEAD (same as GT)
	cmd := exec.Command("git", "-C", repoPath, "worktree", "add", "--detach", targetPath, branch)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	// Set a 2-minute timeout for worktree operation
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to create worktree: %v", err)
		}
		return nil
	case <-time.After(2 * time.Minute):
		cmd.Process.Kill()
		return fmt.Errorf("worktree operation timed out after 2 minutes")
	}
}

// ensureGitignore adds .worktrees/ to .gitignore if not already present
func ensureGitignore(repoPath string) error {
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	entry := ".worktrees/"

	// Read existing .gitignore
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read .gitignore: %v", err)
	}

	// Check if entry already exists
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			return nil // Already present
		}
	}

	// Append entry
	var newContent string
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		newContent = string(content) + "\n" + entry + "\n"
	} else {
		newContent = string(content) + entry + "\n"
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write .gitignore: %v", err)
	}

	fmt.Printf("📝 Added %s to .gitignore\n", entry)
	return nil
}

// performWorktree handles the worktree creation logic
// If inRepo is true, creates in .worktrees/ inside the repo (GT-style)
// Otherwise, creates in basePath with date prefix (Ruby try style)
func performWorktree(repoPath, branch, basePath string, inRepo bool) (string, error) {
	sanitizedBranch := sanitizeBranchName(branch)

	var targetPath string
	var dirName string

	if inRepo {
		// GT-style: .worktrees/branch-name inside the repo
		worktreesDir := filepath.Join(repoPath, ".worktrees")
		if err := os.MkdirAll(worktreesDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create .worktrees directory: %v", err)
		}

		// Add to .gitignore
		if err := ensureGitignore(repoPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}

		dirName = sanitizedBranch
		targetPath = filepath.Join(worktreesDir, sanitizedBranch)
	} else {
		// Ruby try style: date-prefix in TRY_PATH
		datePrefix := time.Now().Format("2006-01-02")
		dirName = fmt.Sprintf("%s-%s", datePrefix, sanitizedBranch)
		targetPath = filepath.Join(basePath, dirName)
	}

	// Check if target already exists
	if _, err := os.Stat(targetPath); err == nil {
		if inRepo {
			return "", fmt.Errorf("worktree already exists: %s", targetPath)
		}
		// Add a number suffix for non-inRepo mode
		for i := 2; ; i++ {
			testPath := fmt.Sprintf("%s-%d", targetPath, i)
			if _, err := os.Stat(testPath); os.IsNotExist(err) {
				targetPath = testPath
				dirName = fmt.Sprintf("%s-%d", dirName, i)
				break
			}
		}
	}

	// Create the worktree
	fmt.Printf("🌳 Creating worktree for %s in %s...\n", branch, dirName)
	if err := createWorktree(repoPath, targetPath, branch); err != nil {
		return "", err
	}

	return targetPath, nil
}

// handleDirectWorktree handles the CLI worktree command
func handleDirectWorktree(repoArg, branch string, inRepo bool, config *Config) {
	var repoPath string
	var basePath string

	// Determine the repository path
	if repoArg == "." || repoArg == "" {
		// Use current directory
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: couldn't get current directory: %v\n", err)
			os.Exit(1)
		}
		foundRepo, isRepo := isGitRepository(cwd)
		if !isRepo {
			fmt.Fprintf(os.Stderr, "Error: not inside a git repository\n")
			os.Exit(1)
		}
		repoPath = foundRepo
	} else {
		// Use specified path
		absPath, err := filepath.Abs(repoArg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid path: %v\n", err)
			os.Exit(1)
		}
		foundRepo, isRepo := isGitRepository(absPath)
		if !isRepo {
			fmt.Fprintf(os.Stderr, "Error: %s is not a git repository\n", repoArg)
			os.Exit(1)
		}
		repoPath = foundRepo
	}

	// Get base path for non-inRepo mode
	if !inRepo {
		basePath = getDefaultPath(config)
		if basePath == "" {
			basePath = promptForPath()
			var err error
			config, err = getResolvedConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to reload config: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Perform the worktree creation
	targetPath, err := performWorktree(repoPath, branch, basePath, inRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Change to the worktree directory
	if err := os.Chdir(targetPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't change directory: %v\n", err)
		os.Exit(1)
	}

	// Launch a new shell
	shell := getShell(config)

	fmt.Printf("\n🌳 Worktree created and entering %s\n\n", filepath.Base(targetPath))

	cmd := exec.Command(shell)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = targetPath

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error launching shell: %v\n", err)
		os.Exit(1)
	}
}

// Shell init functions

// detectUserShell determines the user's shell for wrapper generation
// Priority: TRY_SHELL env > config file > $SHELL > default
func detectUserShell(config *Config) string {
	// Check TRY_SHELL environment variable
	if tryShell := os.Getenv("TRY_SHELL"); tryShell != "" {
		return filepath.Base(tryShell)
	}

	// Check config file
	if config != nil && config.Shell != "" {
		return filepath.Base(config.Shell)
	}

	// Check $SHELL environment variable
	if shell := os.Getenv("SHELL"); shell != "" {
		return filepath.Base(shell)
	}

	// Default to bash
	return "bash"
}

// generateBashZshWrapper generates a shell wrapper for bash or zsh
func generateBashZshWrapper(tryPath string) string {
	return fmt.Sprintf(`# try shell wrapper - add this to your .bashrc or .zshrc
try() {
    local dir
    dir=$(%s --select-only "$@")
    if [[ $? -eq 0 && -n "$dir" ]]; then
        cd "$dir" || return 1
    fi
}
`, tryPath)
}

// generateFishWrapper generates a shell wrapper for fish
func generateFishWrapper(tryPath string) string {
	return fmt.Sprintf(`# try shell wrapper - add this to your config.fish
function try
    set -l dir (%s --select-only $argv)
    if test $status -eq 0 -a -n "$dir"
        cd $dir
    end
end
`, tryPath)
}

// handleInitCommand handles the 'try init' command
func handleInitCommand(customPath string, config *Config) {
	// Determine the path to the try binary
	var tryPath string
	if customPath != "" {
		absPath, err := filepath.Abs(customPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid path: %v\n", err)
			os.Exit(1)
		}
		tryPath = absPath
	} else {
		// Try to find the current executable
		execPath, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: couldn't determine executable path: %v\n", err)
			fmt.Fprintln(os.Stderr, "Please specify the path: try init /path/to/try")
			os.Exit(1)
		}
		tryPath, err = filepath.EvalSymlinks(execPath)
		if err != nil {
			tryPath = execPath
		}
	}

	// Detect shell
	shell := detectUserShell(config)

	fmt.Printf("🐚 Detected shell: %s\n\n", shell)

	var wrapper string
	var configFile string

	switch shell {
	case "fish":
		wrapper = generateFishWrapper(tryPath)
		home, _ := os.UserHomeDir()
		configFile = filepath.Join(home, ".config", "fish", "config.fish")
	case "zsh":
		wrapper = generateBashZshWrapper(tryPath)
		home, _ := os.UserHomeDir()
		configFile = filepath.Join(home, ".zshrc")
	default: // bash and others
		wrapper = generateBashZshWrapper(tryPath)
		home, _ := os.UserHomeDir()
		configFile = filepath.Join(home, ".bashrc")
	}

	fmt.Println("Add the following to your shell configuration:")
	fmt.Println()
	fmt.Println(wrapper)
	fmt.Printf("Suggested config file: %s\n\n", dimStyle.Render(configFile))
	fmt.Println("After adding the wrapper, restart your shell or run:")
	fmt.Printf("  source %s\n\n", configFile)
	fmt.Println("Then you can use 'try' to change directories instead of spawning a subshell.")
}

func (m model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Clamp to sane minimums to avoid rendering issues on zero-sized TTYs.
		if msg.Width > 0 {
			m.width = msg.Width
		} else {
			m.width = 1
		}
		if msg.Height > 0 {
			m.height = msg.Height
		} else {
			m.height = 1
		}

	case tea.KeyMsg:
		// Handle input mode for new directory name
		if m.inputMode {
			switch msg.String() {
			case "ctrl+c", "esc":
				m.inputMode = false
				m.newName = ""
				m.cursorPos = 0

			case "enter":
				if m.newName != "" {
					datePrefix := time.Now().Format("2006-01-02")
					finalName := fmt.Sprintf("%s-%s", datePrefix, strings.ReplaceAll(m.newName, " ", "-"))
					fullPath := filepath.Join(m.basePath, finalName)
					m.selected = &selection{
						Type: "mkdir",
						Path: fullPath,
					}
					m.quitting = true
					return m, tea.Quit
				}

			case "backspace":
				m.newName, m.cursorPos = deleteCharBackward(m.newName, m.cursorPos)

			// Text editing shortcuts
			case "ctrl+a":
				m.cursorPos = 0
			case "ctrl+e":
				m.cursorPos = runeLen(m.newName)
			case "ctrl+b", "left":
				if m.cursorPos > 0 {
					m.cursorPos--
				}
			case "ctrl+f", "right":
				if m.cursorPos < runeLen(m.newName) {
					m.cursorPos++
				}
			case "ctrl+k":
				m.newName = deleteToEnd(m.newName, m.cursorPos)
			case "ctrl+w":
				m.newName, m.cursorPos = deleteWordBackward(m.newName, m.cursorPos)

			default:
				// Handle character input (including paste)
				switch msg.Type {
				case tea.KeyRunes:
					for _, r := range msg.Runes {
						m.newName, m.cursorPos = insertCharAt(m.newName, m.cursorPos, r)
					}
				}
			}
			return m, nil
		}

		// Handle delete confirmation mode - requires typing "YES"
		if m.confirmDelete && m.deleteTarget != nil {
			switch msg.String() {
			case "ctrl+c", "esc":
				m.confirmDelete = false
				m.deleteTarget = nil
				m.deleteConfirmBuffer = ""

			case "backspace":
				if len(m.deleteConfirmBuffer) > 0 {
					m.deleteConfirmBuffer = m.deleteConfirmBuffer[:len(m.deleteConfirmBuffer)-1]
				}

			default:
				// Handle character input
				if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
					char := string(msg.Runes[0])
					expectedChars := "YES"
					bufLen := len(m.deleteConfirmBuffer)

					// Check if the character matches the expected next character
					if bufLen < 3 && char == string(expectedChars[bufLen]) {
						m.deleteConfirmBuffer += char

						// Check if confirmation is complete
						if m.deleteConfirmBuffer == "YES" {
							// Perform deletion
							if err := os.RemoveAll(m.deleteTarget.Path); err != nil {
								m.confirmDelete = false
								m.deleteTarget = nil
								m.deleteConfirmBuffer = ""
								return m, nil
							}
							// Reload directories and reset state
							m.loadTries()
							m.filterTries()
							m.confirmDelete = false
							m.deleteTarget = nil
							m.deleteConfirmBuffer = ""
							// Adjust cursor if it's out of bounds
							if m.cursor >= len(m.filteredTries) {
								m.cursor = len(m.filteredTries) - 1
								if m.cursor < 0 {
									m.cursor = 0
								}
							}
						}
					} else {
						// Wrong character - cancel
						m.confirmDelete = false
						m.deleteTarget = nil
						m.deleteConfirmBuffer = ""
					}
				} else {
					// Non-character key - cancel
					m.confirmDelete = false
					m.deleteTarget = nil
					m.deleteConfirmBuffer = ""
				}
			}
			return m, nil
		}

		// Handle rename mode
		if m.renameMode && m.renameTarget != nil {
			switch msg.String() {
			case "ctrl+c", "esc":
				m.renameMode = false
				m.renameTarget = nil
				m.renameName = ""
				m.cursorPos = 0

			case "enter":
				// Validate and perform rename
				if m.renameName != "" && m.renameName != m.renameTarget.Basename {
					// Check for invalid characters
					if strings.Contains(m.renameName, "/") || strings.Contains(m.renameName, "\\") {
						// Invalid name - stay in rename mode (could show error)
						return m, nil
					}
					// Check if target already exists
					newPath := filepath.Join(m.basePath, m.renameName)
					if _, err := os.Stat(newPath); err == nil {
						// Already exists - stay in rename mode
						return m, nil
					}
					// Proceed with rename
					m.selected = &selection{
						Type:    "rename",
						Path:    newPath,
						OldPath: m.renameTarget.Path,
					}
					m.quitting = true
					return m, tea.Quit
				}
				// Empty or same name - just exit rename mode
				m.renameMode = false
				m.renameTarget = nil
				m.renameName = ""
				m.cursorPos = 0

			case "backspace":
				m.renameName, m.cursorPos = deleteCharBackward(m.renameName, m.cursorPos)

			// Text editing shortcuts
			case "ctrl+a":
				m.cursorPos = 0
			case "ctrl+e":
				m.cursorPos = runeLen(m.renameName)
			case "ctrl+b", "left":
				if m.cursorPos > 0 {
					m.cursorPos--
				}
			case "ctrl+f", "right":
				if m.cursorPos < runeLen(m.renameName) {
					m.cursorPos++
				}
			case "ctrl+k":
				m.renameName = deleteToEnd(m.renameName, m.cursorPos)
			case "ctrl+w":
				m.renameName, m.cursorPos = deleteWordBackward(m.renameName, m.cursorPos)

			default:
				// Handle character input
				switch msg.Type {
				case tea.KeyRunes:
					for _, r := range msg.Runes {
						// Don't allow / or \ in names
						if r != '/' && r != '\\' {
							m.renameName, m.cursorPos = insertCharAt(m.renameName, m.cursorPos, r)
						}
					}
				}
			}
			return m, nil
		}

		// Normal mode
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit

		case "ctrl+n":
			// Quick create new experiment or clone
			if m.searchTerm != "" {
				// Check if it's a GitHub URL
				isGH, cloneURL := isGitHubURL(m.searchTerm)
				if isGH {
					// Clone repository
					repoName := extractRepoName(cloneURL)
					datePrefix := time.Now().Format("2006-01-02")
					finalName := fmt.Sprintf("%s-%s", datePrefix, repoName)
					fullPath := filepath.Join(m.basePath, finalName)
					m.selected = &selection{
						Type:     "clone",
						Path:     fullPath,
						CloneURL: cloneURL,
					}
					m.quitting = true
					return m, tea.Quit
				} else {
					// Regular create
					datePrefix := time.Now().Format("2006-01-02")
					finalName := fmt.Sprintf("%s-%s", datePrefix, strings.ReplaceAll(m.searchTerm, " ", "-"))
					fullPath := filepath.Join(m.basePath, finalName)
					m.selected = &selection{
						Type: "mkdir",
						Path: fullPath,
					}
					m.quitting = true
					return m, tea.Quit
				}
			} else {
				// Enter input mode for new name
				m.inputMode = true
				m.newName = ""
				m.cursorPos = 0
			}

		case "ctrl+t":
			// Quick create - same as Ctrl+N but more memorable shortcut
			if m.searchTerm != "" {
				// Check if it's a GitHub URL
				isGH, cloneURL := isGitHubURL(m.searchTerm)
				if isGH {
					repoName := extractRepoName(cloneURL)
					datePrefix := time.Now().Format("2006-01-02")
					finalName := fmt.Sprintf("%s-%s", datePrefix, repoName)
					fullPath := filepath.Join(m.basePath, finalName)
					m.selected = &selection{
						Type:     "clone",
						Path:     fullPath,
						CloneURL: cloneURL,
					}
					m.quitting = true
					return m, tea.Quit
				}
				// Regular create
				datePrefix := time.Now().Format("2006-01-02")
				finalName := fmt.Sprintf("%s-%s", datePrefix, strings.ReplaceAll(m.searchTerm, " ", "-"))
				fullPath := filepath.Join(m.basePath, finalName)
				m.selected = &selection{
					Type: "mkdir",
					Path: fullPath,
				}
				m.quitting = true
				return m, tea.Quit
			}
			// Enter input mode for new name
			m.inputMode = true
			m.newName = ""
			m.cursorPos = 0

		case "ctrl+d", "delete":
			// Delete directory with confirmation
			if m.cursor < len(m.filteredTries) {
				m.confirmDelete = true
				m.deleteConfirmBuffer = ""
				entry := m.filteredTries[m.cursor]
				m.deleteTarget = &entry
			}

		case "ctrl+r":
			// Rename directory
			if m.cursor < len(m.filteredTries) {
				entry := m.filteredTries[m.cursor]
				m.renameMode = true
				m.renameTarget = &entry
				m.renameName = entry.Basename
				m.cursorPos = runeLen(entry.Basename) // Cursor at end
			}

		case "enter":
			if m.cursor < len(m.filteredTries) {
				// Select existing directory
				m.selected = &selection{
					Type: "cd",
					Path: m.filteredTries[m.cursor].Path,
				}
				m.quitting = true
				return m, tea.Quit
			} else if m.cursor == len(m.filteredTries) {
				// Create new directory or clone repository
				if m.searchTerm != "" {
					// Check if it's a GitHub URL
					isGH, cloneURL := isGitHubURL(m.searchTerm)
					if isGH {
						// Clone repository
						repoName := extractRepoName(cloneURL)
						datePrefix := time.Now().Format("2006-01-02")
						finalName := fmt.Sprintf("%s-%s", datePrefix, repoName)
						fullPath := filepath.Join(m.basePath, finalName)
						m.selected = &selection{
							Type:     "clone",
							Path:     fullPath,
							CloneURL: cloneURL,
						}
						m.quitting = true
						return m, tea.Quit
					} else {
						// Regular create
						datePrefix := time.Now().Format("2006-01-02")
						finalName := fmt.Sprintf("%s-%s", datePrefix, strings.ReplaceAll(m.searchTerm, " ", "-"))
						fullPath := filepath.Join(m.basePath, finalName)
						m.selected = &selection{
							Type: "mkdir",
							Path: fullPath,
						}
						m.quitting = true
						return m, tea.Quit
					}
				} else {
					// Enter input mode for new name
					m.inputMode = true
					m.newName = ""
					m.cursorPos = 0
				}
			}

		case "up", "ctrl+p", "ctrl+k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}

		case "down", "ctrl+j":
			totalItems := len(m.filteredTries) + 1
			if m.cursor < totalItems-1 {
				m.cursor++
				m.adjustScroll()
			}

		case "backspace":
			if len(m.searchTerm) > 0 {
				m.searchTerm, m.cursorPos = deleteCharBackward(m.searchTerm, m.cursorPos)
				m.filterTries()
				m.cursor = 0
				m.scrollOffset = 0
			}

		case "ctrl+u":
			// Clear search
			m.searchTerm = ""
			m.cursorPos = 0
			m.filterTries()
			m.cursor = 0
			m.scrollOffset = 0

		// Search term text editing shortcuts
		case "ctrl+a":
			m.cursorPos = 0
		case "ctrl+e":
			m.cursorPos = runeLen(m.searchTerm)
		case "ctrl+b", "left":
			if m.cursorPos > 0 {
				m.cursorPos--
			}
		case "ctrl+f", "right":
			if m.cursorPos < runeLen(m.searchTerm) {
				m.cursorPos++
			}
		case "ctrl+w":
			m.searchTerm, m.cursorPos = deleteWordBackward(m.searchTerm, m.cursorPos)
			m.filterTries()
			m.cursor = 0
			m.scrollOffset = 0

		default:
			// Handle character input for search (including paste)
			switch msg.Type {
			case tea.KeyRunes:
				// This handles both single chars and pasted content
				input := string(msg.Runes)
				if isValidSearchInput(input) {
					m.searchTerm, m.cursorPos = insertStringAt(m.searchTerm, m.cursorPos, input)
					m.filterTries()
					m.cursor = 0
					m.scrollOffset = 0
				}
			}
		}
	}

	return m, nil
}

func (m *model) adjustScroll() {
	maxVisible := m.height - 10
	if maxVisible < 3 {
		maxVisible = 3
	}

	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+maxVisible {
		m.scrollOffset = m.cursor - maxVisible + 1
	}
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("📁 Try - Quick Experiment Directories"))
	b.WriteString("\n")

	// Handle delete confirmation mode
	if m.confirmDelete && m.deleteTarget != nil {
		b.WriteString("\n")
		b.WriteString(dangerStyle.Render("⚠️  Delete Directory"))
		b.WriteString("\n\n")
		b.WriteString("Are you sure you want to delete this directory?\n\n")
		b.WriteString(warningStyle.Render("  " + m.deleteTarget.Name))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  " + m.deleteTarget.Path))
		b.WriteString("\n\n")
		b.WriteString(dangerStyle.Render("This action cannot be undone!"))
		b.WriteString("\n\n")
		// Show YES confirmation with typed progress
		b.WriteString("Type ")
		b.WriteString(dangerStyle.Render("YES"))
		b.WriteString(" to confirm: ")
		for i, char := range "YES" {
			if i < len(m.deleteConfirmBuffer) {
				b.WriteString(createNewStyle.Render(string(char)))
			} else if i == len(m.deleteConfirmBuffer) {
				b.WriteString(cursorStyle.Render(string(char)))
			} else {
				b.WriteString(dimStyle.Render(string(char)))
			}
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("ESC: Cancel"))
		return b.String()
	}

	// Handle rename mode
	if m.renameMode && m.renameTarget != nil {
		b.WriteString("\n")
		b.WriteString(promptStyle.Render("Rename directory:"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("From: "))
		b.WriteString(warningStyle.Render(m.renameTarget.Basename))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("To:   "))
		b.WriteString(renderTextWithCursor(m.renameName, m.cursorPos, searchInputStyle, cursorStyle))
		b.WriteString("\n")
		// Show validation messages
		if m.renameName == "" {
			b.WriteString("\n")
			b.WriteString(dangerStyle.Render("Name cannot be empty"))
		} else if m.renameName == m.renameTarget.Basename {
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("(no change)"))
		} else if strings.Contains(m.renameName, "/") || strings.Contains(m.renameName, "\\") {
			b.WriteString("\n")
			b.WriteString(dangerStyle.Render("Name cannot contain / or \\"))
		} else {
			newPath := filepath.Join(m.basePath, m.renameName)
			if _, err := os.Stat(newPath); err == nil {
				b.WriteString("\n")
				b.WriteString(dangerStyle.Render("A directory with this name already exists"))
			}
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Enter: Rename  Ctrl-A/E: Start/End  Ctrl-K: Delete to end  ESC: Cancel"))
		return b.String()
	}

	// Handle input mode for new directory
	if m.inputMode {
		b.WriteString("\n")
		b.WriteString(promptStyle.Render("New directory name:"))
		b.WriteString("\n")
		datePrefix := time.Now().Format("2006-01-02")
		b.WriteString(dimStyle.Render(datePrefix + "-"))
		b.WriteString(renderTextWithCursor(m.newName, m.cursorPos, searchInputStyle, cursorStyle))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Enter: Create  Ctrl-A/E: Start/End  Ctrl-K: Delete to end  ESC: Cancel"))
		return b.String()
	}

	b.WriteString(m.separatorLine())
	b.WriteString("\n")

	// Search input with cursor
	b.WriteString(searchStyle.Render("Search: "))
	if m.searchTerm == "" {
		b.WriteString(cursorStyle.Render(" "))
		b.WriteString(dimStyle.Render("(type to filter)"))
	} else {
		b.WriteString(renderTextWithCursor(m.searchTerm, m.cursorPos, searchInputStyle, cursorStyle))
	}
	b.WriteString("\n")
	b.WriteString(m.separatorLine())
	b.WriteString("\n")

	// Calculate visible window (accounting for extra help lines and separators)
	maxVisible := m.height - 10
	if maxVisible < 3 {
		maxVisible = 3
	}
	totalItems := len(m.filteredTries) + 1

	// Display items
	visibleEnd := m.scrollOffset + maxVisible
	if visibleEnd > totalItems {
		visibleEnd = totalItems
	}

	for idx := m.scrollOffset; idx < visibleEnd; idx++ {
		// Add blank line before "Create new"
		if idx == len(m.filteredTries) && len(m.filteredTries) > 0 {
			b.WriteString("\n")
		}

		// Cursor
		isSelected := idx == m.cursor
		if isSelected {
			b.WriteString(cursorStyle.Render("→ "))
		} else {
			b.WriteString("  ")
		}

		// Display entry
		if idx < len(m.filteredTries) {
			entry := m.filteredTries[idx]
			line := m.formatEntry(entry, isSelected)
			b.WriteString(line)
		} else {
			// Create new option
			line := m.formatCreateNew(isSelected)
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	// Scroll indicator
	if totalItems > maxVisible {
		b.WriteString(m.separatorLine())
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("[%d-%d/%d]", m.scrollOffset+1, visibleEnd, totalItems)))
		b.WriteString("\n")
	}

	b.WriteString(m.separatorLine())
	b.WriteString("\n")
	// Navigation hints
	b.WriteString(helpStyle.Render("↑↓: Navigate  Enter: Select  Ctrl+N/T: New  Ctrl+R: Rename  Ctrl+D: Delete"))
	b.WriteString("\n")
	// Action hints
	b.WriteString(helpStyle.Render("Ctrl+A/E/B/F/W: Edit text  Ctrl+U: Clear  ESC: Quit"))

	return b.String()
}

func (m model) separatorLine() string {
	width := m.width - 1
	if width < 0 {
		width = 0
	}
	return separatorStyle.Render(strings.Repeat("─", width))
}

func (m model) formatEntry(entry tryEntry, isSelected bool) string {
	var result strings.Builder

	// Icon
	result.WriteString("📁 ")

	// Parse and format the name
	name := entry.Basename
	var displayName string

	if parts := strings.SplitN(name, "-", 4); len(parts) >= 4 &&
		len(parts[0]) == 4 && len(parts[1]) == 2 && len(parts[2]) == 2 {
		// Date-prefixed format
		datePart := strings.Join(parts[:3], "-")
		namePart := strings.Join(parts[3:], "-")

		if isSelected {
			displayName = selectedStyle.Render(
				dateStyle.Render(datePart) +
					dimStyle.Render("-") +
					m.highlightMatches(namePart))
		} else {
			displayName = dateStyle.Render(datePart) +
				dimStyle.Render("-") +
				m.highlightMatches(namePart)
		}
	} else {
		// Regular name
		if isSelected {
			displayName = selectedStyle.Render(m.highlightMatches(name))
		} else {
			displayName = m.highlightMatches(name)
		}
	}

	result.WriteString(displayName)

	// Add metadata (time and score)
	timeText := m.formatRelativeTime(entry.MTime)
	scoreText := fmt.Sprintf("%.1f", entry.Score)
	metaText := fmt.Sprintf(" %s, score: %s", timeText, scoreText)

	// Calculate padding
	plainTextLen := len(entry.Basename) + 2 // +2 for emoji
	metaLen := len(metaText)
	paddingNeeded := m.width - 2 - plainTextLen - metaLen // -2 for cursor space
	if paddingNeeded > 0 {
		result.WriteString(strings.Repeat(" ", paddingNeeded))
	}

	result.WriteString(dimStyle.Render(metaText))

	return result.String()
}

func (m model) formatCreateNew(isSelected bool) string {
	var result strings.Builder
	var displayText string
	var iconLen int

	// Check if search term is a GitHub URL
	isGH, cloneURL := isGitHubURL(m.searchTerm)

	if isGH {
		result.WriteString("📦 ")
		iconLen = 2
		repoName := extractRepoName(cloneURL)
		displayText = fmt.Sprintf("Clone: %s", repoName)
		if isSelected {
			result.WriteString(selectedStyle.Render(createNewStyle.Render(displayText)))
		} else {
			result.WriteString(createNewStyle.Render(displayText))
		}
	} else {
		result.WriteString("✨ ")
		iconLen = 2
		if m.searchTerm == "" {
			displayText = "Create new experiment..."
		} else {
			displayText = fmt.Sprintf("Create: %s", m.searchTerm)
		}
		if isSelected {
			result.WriteString(selectedStyle.Render(createNewStyle.Render(displayText)))
		} else {
			result.WriteString(createNewStyle.Render(displayText))
		}
	}

	// Padding
	textLen := len(displayText) + iconLen
	paddingNeeded := m.width - 2 - textLen // -2 for cursor space
	if paddingNeeded > 0 {
		result.WriteString(strings.Repeat(" ", paddingNeeded))
	}

	return result.String()
}

func (m model) highlightMatches(text string) string {
	if m.searchTerm == "" {
		return text
	}

	var result strings.Builder
	queryLower := strings.ToLower(m.searchTerm)
	queryChars := []rune(queryLower)
	queryIdx := 0

	for _, char := range text {
		if queryIdx < len(queryChars) && strings.ToLower(string(char)) == string(queryChars[queryIdx]) {
			result.WriteString(matchStyle.Render(string(char)))
			queryIdx++
		} else {
			result.WriteString(string(char))
		}
	}

	return result.String()
}

func (m model) formatRelativeTime(t time.Time) string {
	duration := time.Since(t)

	switch {
	case duration < 10*time.Second:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(duration.Hours()))
	case duration < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(duration.Hours()/24))
	case duration < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(duration.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(duration.Hours()/(24*365)))
	}
}

func handleDirectClone(url string, config *Config) {
	// Validate it's a GitHub URL
	isGH, cloneURL := isGitHubURL(url)
	if !isGH {
		fmt.Fprintf(os.Stderr, "Error: Not a valid GitHub URL: %s\n", url)
		os.Exit(1)
	}

	// Get base path
	basePath := getDefaultPath(config)
	if basePath == "" {
		basePath = promptForPath()
		// Reload config after prompting
		var err error
		config, err = getResolvedConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to reload config after setting path: %v\n", err)
			os.Exit(1)
		}
	}

	// Perform the clone
	fullPath, err := performClone(cloneURL, basePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Change to the directory
	if err := os.Chdir(fullPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't change directory: %v\n", err)
		os.Exit(1)
	}

	// Launch a new shell
	shell := getShell(config)

	fmt.Printf("\n✨ Successfully cloned and entering %s\n\n", filepath.Base(fullPath))

	cmd := exec.Command(shell)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = fullPath

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error launching shell: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	// Simple argument parsing
	searchTerm := ""
	showHelp := false
	showVersion := false
	cloneURL := ""
	selectOnly := false
	// Worktree mode
	worktreeMode := false
	worktreeRepo := ""
	worktreeBranch := ""
	worktreeInRepo := false
	// Init command
	initMode := false
	initPath := ""

	args := os.Args[1:]

	// Check for worktree patterns first
	// try worktree <branch>
	// try worktree --in-repo <branch>
	// try . <branch>
	// try . --in-repo <branch>
	// try ./path <branch>
	if len(args) >= 2 {
		firstArg := args[0]
		if firstArg == "worktree" || firstArg == "." || strings.HasPrefix(firstArg, "./") || strings.HasPrefix(firstArg, "/") {
			worktreeMode = true
			if firstArg == "worktree" {
				worktreeRepo = "."
			} else {
				worktreeRepo = firstArg
			}

			// Parse remaining args
			for i := 1; i < len(args); i++ {
				arg := args[i]
				if arg == "--in-repo" {
					worktreeInRepo = true
				} else if !strings.HasPrefix(arg, "-") && worktreeBranch == "" {
					worktreeBranch = arg
				}
			}

			if worktreeBranch == "" {
				fmt.Fprintln(os.Stderr, "Error: worktree requires a branch name")
				os.Exit(1)
			}
		}
	}

	// Check for init command
	if len(args) >= 1 && args[0] == "init" {
		initMode = true
		if len(args) >= 2 && !strings.HasPrefix(args[1], "-") {
			initPath = args[1]
		}
	}

	// Standard argument parsing (if not worktree or init mode)
	if !worktreeMode && !initMode {
		for i := 0; i < len(args); i++ {
			arg := args[i]
			switch arg {
			case "--help", "-h", "help":
				showHelp = true
			case "--version", "-v":
				showVersion = true
			case "--select-only", "-s":
				selectOnly = true
			case "--clone", "-c":
				// Get the next argument as the URL
				if i+1 < len(args) {
					cloneURL = args[i+1]
					i++ // Skip the URL argument
				} else {
					fmt.Fprintln(os.Stderr, "Error: --clone requires a URL argument")
					os.Exit(1)
				}
			default:
				if !strings.HasPrefix(arg, "-") {
					searchTerm += arg + " "
				}
			}
		}
	}

	// Handle version flag early (doesn't need config)
	if showVersion {
		fmt.Printf("try version %s\n", version)
		return
	}

	// Load config once at startup
	config, err := getResolvedConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if showHelp {
		printHelp(config)
		return
	}

	// Handle init command
	if initMode {
		handleInitCommand(initPath, config)
		return
	}

	// Handle worktree operation
	if worktreeMode {
		handleDirectWorktree(worktreeRepo, worktreeBranch, worktreeInRepo, config)
		return
	}

	// Handle direct clone operation
	if cloneURL != "" {
		handleDirectClone(cloneURL, config)
		return
	}

	searchTerm = strings.TrimSpace(searchTerm)

	// Check if we have a TTY
	if !checkTTYRequirements(selectOnly) {
		fmt.Fprintln(os.Stderr, "Error: try requires an interactive terminal")
		os.Exit(1)
	}

	// Run the TUI
	m := initialModel(searchTerm, config)
	tty, ttyErr := openTTY()
	var p *tea.Program
	if ttyErr != nil {
		if selectOnly {
			lipgloss.SetColorProfile(termenv.ANSI256)
			p = tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(os.Stderr))
		} else {
			p = tea.NewProgram(m, tea.WithAltScreen())
		}
	} else {
		defer tty.Close()
		if selectOnly {
			lipgloss.SetColorProfile(termenv.ANSI256)
		}
		p = tea.NewProgram(m, tea.WithAltScreen(), tea.WithInput(tty), tea.WithOutput(tty))
	}

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var ok bool
	m, ok = finalModel.(model)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unexpected model type returned\n")
		os.Exit(1)
	}

	// Handle the selection
	if m.selected != nil {
		switch m.selected.Type {
		case "cd":
			// Touch the directory to update access time
			if err := os.Chtimes(m.selected.Path, time.Now(), time.Now()); err != nil {
				// Non-fatal, just log it
				if !selectOnly {
					fmt.Fprintf(os.Stderr, "Warning: couldn't update access time: %v\n", err)
				}
			}

			if selectOnly {
				// Just output the path and exit
				fmt.Println(m.selected.Path)
				os.Exit(0)
			}

			// Change to the directory
			if err := os.Chdir(m.selected.Path); err != nil {
				fmt.Fprintf(os.Stderr, "Error: couldn't change directory: %v\n", err)
				os.Exit(1)
			}

			// Launch a new shell in the selected directory
			shell := getShell(m.config)

			fmt.Printf("\n🚀 Entering %s\n\n", filepath.Base(m.selected.Path))

			cmd := exec.Command(shell)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Dir = m.selected.Path

			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error launching shell: %v\n", err)
				os.Exit(1)
			}

		case "mkdir":
			// Create the new directory
			if err := os.MkdirAll(m.selected.Path, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
				os.Exit(1)
			}

			// Touch it
			if err := os.Chtimes(m.selected.Path, time.Now(), time.Now()); err != nil {
				// Non-fatal, just log it
				if !selectOnly {
					fmt.Fprintf(os.Stderr, "Warning: couldn't update access time: %v\n", err)
				}
			}

			if selectOnly {
				// Just output the path and exit
				fmt.Println(m.selected.Path)
				os.Exit(0)
			}

			// Change to it
			if err := os.Chdir(m.selected.Path); err != nil {
				fmt.Fprintf(os.Stderr, "Error: couldn't change directory: %v\n", err)
				os.Exit(1)
			}

			// Launch a new shell
			shell := getShell(m.config)

			fmt.Printf("\n✨ Created and entering %s\n\n", filepath.Base(m.selected.Path))

			cmd := exec.Command(shell)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Dir = m.selected.Path

			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error launching shell: %v\n", err)
				os.Exit(1)
			}

		case "clone":
			// Clone GitHub repository
			cloneURL := m.selected.CloneURL

			// Perform the clone
			targetPath, err := performClone(cloneURL, m.basePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if selectOnly {
				// Just output the path and exit
				fmt.Println(targetPath)
				os.Exit(0)
			}

			// Change to the directory
			if err := os.Chdir(targetPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error: couldn't change directory: %v\n", err)
				os.Exit(1)
			}

			// Launch a new shell
			shell := getShell(m.config)

			fmt.Printf("\n✨ Successfully cloned and entering %s\n\n", filepath.Base(targetPath))

			cmd := exec.Command(shell)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Dir = targetPath

			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error launching shell: %v\n", err)
				os.Exit(1)
			}

		case "rename":
			// Rename directory
			if err := os.Rename(m.selected.OldPath, m.selected.Path); err != nil {
				fmt.Fprintf(os.Stderr, "Error renaming directory: %v\n", err)
				os.Exit(1)
			}

			// Touch the renamed directory
			if err := os.Chtimes(m.selected.Path, time.Now(), time.Now()); err != nil {
				if !selectOnly {
					fmt.Fprintf(os.Stderr, "Warning: couldn't update access time: %v\n", err)
				}
			}

			if selectOnly {
				fmt.Println(m.selected.Path)
				os.Exit(0)
			}

			// Change to the renamed directory
			if err := os.Chdir(m.selected.Path); err != nil {
				fmt.Fprintf(os.Stderr, "Error: couldn't change directory: %v\n", err)
				os.Exit(1)
			}

			// Launch a new shell
			shell := getShell(m.config)

			fmt.Printf("\n📝 Renamed to %s\n\n", filepath.Base(m.selected.Path))

			cmd := exec.Command(shell)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Dir = m.selected.Path

			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error launching shell: %v\n", err)
				os.Exit(1)
			}
		}
	}
}

func printHelp(config *Config) {
	basePath := getDefaultPath(config)
	if basePath == "" {
		basePath = "Not configured (will prompt on first use)"
	}
	shellInfo := ""
	if config != nil && config.Shell != "" {
		shellInfo = fmt.Sprintf("\n  Shell override: %s", config.Shell)
	}
	configPath := getConfigPath()
	help := fmt.Sprintf(`📁 try - Quick Experiment Directories

A beautiful TUI for managing lightweight experiment directories.
Perfect for people with ADHD who need quick, organized workspaces.

USAGE:
  try [search_term]           Launch selector with optional search
  try --select-only, -s       Output selected path instead of launching shell
  try --clone <github-url>    Clone a GitHub repository
  try --version, -v           Show version information
  try --help                  Show this help

GIT WORKTREE:
  try worktree <branch>       Create worktree in TRY_PATH (date-prefixed)
  try . <branch>              Shorthand for worktree (current repo)
  try ./path <branch>         Create worktree from specific repo
  try worktree --in-repo <b>  Create in .worktrees/ inside repo (GT-style)
  try . --in-repo <branch>    Shorthand with GT-style location

SHELL INTEGRATION:
  try init                    Generate shell wrapper for cd integration
  try init /path/to/try       Generate wrapper with custom binary path

FEATURES:
  • Fuzzy search with smart scoring
  • Automatic date prefixing (YYYY-MM-DD)
  • Time-based sorting (recent = higher)
  • GitHub repository cloning
  • Git worktree support (GT-compatible)
  • Directory renaming

NAVIGATION:
  ↑/↓          Navigate entries
  Ctrl+j/k     Navigate entries (vim-style)
  Enter        Select directory or create new
  Ctrl+N/T     Create new experiment (quick)
  Ctrl+D       Delete selected directory (requires typing YES)
  Ctrl+R       Rename selected directory
  Backspace    Delete search character
  Ctrl+U       Clear search
  ESC          Cancel and exit

TEXT EDITING (in search/input modes):
  Ctrl+A       Move cursor to beginning
  Ctrl+E       Move cursor to end
  Ctrl+B/←     Move cursor backward
  Ctrl+F/→     Move cursor forward
  Ctrl+W       Delete word backward

CONFIGURATION:
  Environment variables (override config file):
    TRY_PATH   - Base directory for experiments
    TRY_SHELL  - Shell to use (overrides $SHELL)

  Config file: %s
  Current path: %s%s

EXAMPLES:
  try                                      # Launch selector
  try neural                               # Launch with search for "neural"
  try new project                          # Search for "new project"
  try github.com/user/repo                 # Shows clone option in TUI
  try --clone https://github.com/user/repo # Clone directly
  try -s                                   # Select and output path
  cd $(try -s)                             # Use with cd in current shell
  try . feature/my-branch                  # Create worktree for branch
  try . --in-repo main                     # GT-style worktree in .worktrees/
  try init                                 # Generate shell wrapper

First launch automatically creates the base directory.
Selected directories open in a new shell session.
`, configPath, basePath, shellInfo)

	fmt.Print(help)
}

func isatty(fd uintptr) bool {
	// Simple check for TTY
	var stat fs.FileInfo
	file := os.NewFile(fd, "")
	if file == nil {
		return false
	}
	stat, err := file.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

// checkTTYRequirements validates TTY requirements based on mode
func checkTTYRequirements(selectOnly bool) bool {
	if selectOnly {
		// For select-only mode, we only need stdin and stderr to be TTY (stdout goes to pipe)
		return isatty(os.Stdin.Fd()) && isatty(os.Stderr.Fd())
	}
	// Normal mode requires stdin and stdout to be TTY
	return isatty(os.Stdin.Fd()) && isatty(os.Stdout.Fd())
}

// openTTY opens the controlling terminal for stable input/output handling.
// Falls back to the existing stdin/stdout if unavailable.
func openTTY() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}
