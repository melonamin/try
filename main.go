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
	defaultShell     = "/bin/bash"
	defaultTriesDir  = "src/tries"
	configFileName   = "config"
	configDirName    = ".config/try"
)

type Config struct {
	Path  string `json:"path"`
	Shell string `json:"shell,omitempty"`
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
	width         int
	height        int
	quitting      bool
	inputMode     bool
	newName       string
	confirmDelete bool
	deleteTarget  *tryEntry
}

type selection struct {
	Type     string
	Path     string
	CloneURL string // For clone operations
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
	// Use os.UserConfigDir() for platform-appropriate config location:
	// - Linux: ~/.config
	// - macOS: ~/Library/Application Support
	// - Windows: %AppData%
	configHome, err := os.UserConfigDir()
	if err != nil {
		// Fallback to the legacy method if os.UserConfigDir() fails
		home, _ := os.UserHomeDir()
		return filepath.Join(home, configDirName, configFileName)
	}
	
	// Use "try" as the app-specific directory name
	return filepath.Join(configHome, "try", configFileName)
}

func loadConfig() (*Config, error) {
	configPath := getConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
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
	configDir := filepath.Dir(configPath)

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func getDefaultPath() string {
	// First check environment variable
	if basePath := os.Getenv("TRY_PATH"); basePath != "" {
		return basePath
	}

	// Then check stored config
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	if config != nil && config.Path != "" {
		return config.Path
	}

	// No default - will need to prompt
	return ""
}

func getShell(config *Config) string {
	// First check config override
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

	// Expand tilde if present
	if strings.HasPrefix(input, "~/") {
		input = filepath.Join(home, input[2:])
	}

	// Make absolute
	absPath, err := filepath.Abs(input)
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
			// Validate the shell exists
			if _, err := exec.LookPath(shellInput); err == nil {
				config.Shell = shellInput
				fmt.Printf("✅ Shell set to: %s\n", createNewStyle.Render(shellInput))
			} else {
				fmt.Printf("⚠️  Shell '%s' not found, using $SHELL\n", shellInput)
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

func initialModel(searchTerm string) model {
	basePath := getDefaultPath()

	// If no path configured, prompt for it
	if basePath == "" {
		basePath = promptForPath()
	}

	// Ensure base path exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", basePath, err)
	}

	m := model{
		searchTerm: strings.ReplaceAll(searchTerm, " ", "-"),
		basePath:   basePath,
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

func (m model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		// Handle input mode for new directory name
		if m.inputMode {
			switch msg.String() {
			case "ctrl+c", "esc":
				m.inputMode = false
				m.newName = ""

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
				if len(m.newName) > 0 {
					m.newName = m.newName[:len(m.newName)-1]
				}

			default:
				// Handle character input
				if len(msg.String()) == 1 {
					m.newName += msg.String()
				}
			}
			return m, nil
		}

		// Handle delete confirmation mode
		if m.confirmDelete && m.deleteTarget != nil {
			switch msg.String() {
			case "y", "Y":
				// Perform deletion
				if err := os.RemoveAll(m.deleteTarget.Path); err != nil {
					// Could add error handling here, but for now just reset
					m.confirmDelete = false
					m.deleteTarget = nil
					return m, nil
				}
				// Reload directories and reset state
				m.loadTries()
				m.filterTries()
				m.confirmDelete = false
				m.deleteTarget = nil
				// Adjust cursor if it's out of bounds
				if m.cursor >= len(m.filteredTries) {
					m.cursor = len(m.filteredTries) - 1
					if m.cursor < 0 {
						m.cursor = 0
					}
				}
			default:
				// Cancel deletion on any other key
				m.confirmDelete = false
				m.deleteTarget = nil
			}
			return m, nil
		}

		// Normal mode
		switch msg.String() {
		case "ctrl+c", "esc", "q":
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
			}

		case "ctrl+d", "delete":
			// Delete directory with confirmation
			if m.cursor < len(m.filteredTries) {
				m.confirmDelete = true
				entry := m.filteredTries[m.cursor]
				m.deleteTarget = &entry
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
				m.searchTerm = m.searchTerm[:len(m.searchTerm)-1]
				m.filterTries()
				m.cursor = 0
				m.scrollOffset = 0
			}

		case "ctrl+u":
			// Clear search
			m.searchTerm = ""
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
					m.searchTerm += input
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
		b.WriteString(helpStyle.Render("Press 'y' to confirm, any other key to cancel"))
		return b.String()
	}

	// Handle input mode for new directory
	if m.inputMode {
		b.WriteString("\n")
		b.WriteString(promptStyle.Render("New directory name:"))
		b.WriteString("\n")
		datePrefix := time.Now().Format("2006-01-02")
		b.WriteString(dimStyle.Render(datePrefix + "-"))
		b.WriteString(searchInputStyle.Render(m.newName))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Enter: Create  ESC: Cancel"))
		return b.String()
	}

	b.WriteString(separatorStyle.Render(strings.Repeat("─", m.width-1)))
	b.WriteString("\n")

	// Search input
	b.WriteString(searchStyle.Render("Search: "))
	b.WriteString(searchInputStyle.Render(m.searchTerm))
	if m.searchTerm == "" {
		b.WriteString(dimStyle.Render(" (type to filter)"))
	}
	b.WriteString("\n")
	b.WriteString(separatorStyle.Render(strings.Repeat("─", m.width-1)))
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
		b.WriteString(separatorStyle.Render(strings.Repeat("─", m.width-1)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("[%d-%d/%d]", m.scrollOffset+1, visibleEnd, totalItems)))
		b.WriteString("\n")
	}

	b.WriteString(separatorStyle.Render(strings.Repeat("─", m.width-1)))
	b.WriteString("\n")
	// Navigation hints
	b.WriteString(helpStyle.Render("↑↓/Ctrl+j,k: Navigate Enter: Select Ctrl+N: Quick new Ctrl+D: Delete"))
	b.WriteString("\n")
	// Action hints
	b.WriteString(helpStyle.Render("ESC/q: Quit"))

	return b.String()
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

func handleDirectClone(url string) {
	// Validate it's a GitHub URL
	isGH, cloneURL := isGitHubURL(url)
	if !isGH {
		fmt.Fprintf(os.Stderr, "Error: Not a valid GitHub URL: %s\n", url)
		os.Exit(1)
	}
	
	// Get base path
	basePath := getDefaultPath()
	if basePath == "" {
		basePath = promptForPath()
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
	config, _ := loadConfig()
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
	cloneURL := ""
	selectOnly := false

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--help", "-h", "help":
			showHelp = true
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

	if showHelp {
		printHelp()
		return
	}

	// Handle direct clone operation
	if cloneURL != "" {
		handleDirectClone(cloneURL)
		return
	}

	searchTerm = strings.TrimSpace(searchTerm)

	// Check if we have a TTY
	if !checkTTYRequirements(selectOnly) {
		fmt.Fprintln(os.Stderr, "Error: try requires an interactive terminal")
		os.Exit(1)
	}

	// Run the TUI
	m := initialModel(searchTerm)
	var p *tea.Program
	if selectOnly {
		// Output TUI to stderr so stdout can be piped
		// Force colors by setting the color profile globally
		lipgloss.SetColorProfile(termenv.ANSI256)
		p = tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(os.Stderr))
	} else {
		p = tea.NewProgram(m, tea.WithAltScreen())
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
			config, _ := loadConfig()
			shell := getShell(config)

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
			config, _ := loadConfig()
			shell := getShell(config)

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
			config, _ := loadConfig()
			shell := getShell(config)
			
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
		}
	}
}

func printHelp() {
	basePath := getDefaultPath()
	if basePath == "" {
		basePath = "Not configured (will prompt on first use)"
	}
	config, _ := loadConfig()
	shellInfo := ""
	if config.Shell != "" {
		shellInfo = fmt.Sprintf("\n  Shell:    %s", config.Shell)
	}
	help := fmt.Sprintf(`📁 try - Quick Experiment Directories

A beautiful TUI for managing lightweight experiment directories.
Perfect for people with ADHD who need quick, organized workspaces.

USAGE:
  try [search_term]           Launch selector with optional search
  try --select-only, -s       Output selected path instead of launching shell
  try --clone <github-url>    Clone a GitHub repository
  try --help                  Show this help

FEATURES:
  • Fuzzy search with smart scoring
  • Automatic date prefixing (YYYY-MM-DD)
  • Time-based sorting (recent = higher)
  • GitHub repository cloning

NAVIGATION:
  ↑/↓          Navigate entries
  Ctrl+j/k     Navigate entries (vim-style)
  Enter        Select directory or create new
  Ctrl+N       Create new experiment (quick)
  Ctrl+D       Delete selected directory
  Backspace    Delete search character
  Ctrl+U       Clear search
  ESC or q     Cancel and exit

CONFIGURATION:
  Set TRY_PATH environment variable to change base directory
  Current: %s%s

EXAMPLES:
  try                                      # Launch selector
  try neural                               # Launch with search for "neural"
  try new project                          # Search for "new project"
  try github.com/user/repo                 # Shows clone option in TUI
  try --clone https://github.com/user/repo # Clone directly
  try -s                                   # Select and output path
  cd $(try -s)                             # Use with cd in current shell

First launch automatically creates the base directory.
Selected directories open in a new shell session.
`, basePath, shellInfo)

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
