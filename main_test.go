package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Text Editing Helper Tests
// ============================================================================

func TestInsertCharAt(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		pos      int
		char     rune
		wantText string
		wantPos  int
	}{
		{"insert at start", "hello", 0, 'X', "Xhello", 1},
		{"insert at end", "hello", 5, 'X', "helloX", 6},
		{"insert in middle", "hello", 2, 'X', "heXllo", 3},
		{"insert into empty", "", 0, 'X', "X", 1},
		{"insert unicode", "hello", 2, '世', "he世llo", 3},
		{"negative pos clamps to 0", "hello", -5, 'X', "Xhello", 1},
		{"pos beyond length clamps", "hello", 100, 'X', "helloX", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotPos := insertCharAt(tt.text, tt.pos, tt.char)
			if gotText != tt.wantText {
				t.Errorf("insertCharAt() text = %q, want %q", gotText, tt.wantText)
			}
			if gotPos != tt.wantPos {
				t.Errorf("insertCharAt() pos = %d, want %d", gotPos, tt.wantPos)
			}
		})
	}
}

func TestInsertStringAt(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		pos      int
		insert   string
		wantText string
		wantPos  int
	}{
		{"insert at start", "world", 0, "hello ", "hello world", 6},
		{"insert at end", "hello", 5, " world", "hello world", 11},
		{"insert in middle", "helo", 2, "l", "hello", 3},
		{"insert empty string", "hello", 2, "", "hello", 2},
		{"insert into empty", "", 0, "hello", "hello", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotPos := insertStringAt(tt.text, tt.pos, tt.insert)
			if gotText != tt.wantText {
				t.Errorf("insertStringAt() text = %q, want %q", gotText, tt.wantText)
			}
			if gotPos != tt.wantPos {
				t.Errorf("insertStringAt() pos = %d, want %d", gotPos, tt.wantPos)
			}
		})
	}
}

func TestDeleteCharBackward(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		pos      int
		wantText string
		wantPos  int
	}{
		{"delete from middle", "hello", 3, "helo", 2},
		{"delete from end", "hello", 5, "hell", 4},
		{"delete at start (no-op)", "hello", 0, "hello", 0},
		{"delete single char", "X", 1, "", 0},
		{"delete unicode", "he世llo", 3, "hello", 2},
		{"negative pos (no-op)", "hello", -1, "hello", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotPos := deleteCharBackward(tt.text, tt.pos)
			if gotText != tt.wantText {
				t.Errorf("deleteCharBackward() text = %q, want %q", gotText, tt.wantText)
			}
			if gotPos != tt.wantPos {
				t.Errorf("deleteCharBackward() pos = %d, want %d", gotPos, tt.wantPos)
			}
		})
	}
}

func TestDeleteToEnd(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		pos      int
		wantText string
	}{
		{"delete from middle", "hello world", 5, "hello"},
		{"delete from start", "hello", 0, ""},
		{"delete at end (no-op)", "hello", 5, "hello"},
		{"delete beyond end (no-op)", "hello", 100, "hello"},
		{"negative pos clamps", "hello", -1, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deleteToEnd(tt.text, tt.pos)
			if got != tt.wantText {
				t.Errorf("deleteToEnd() = %q, want %q", got, tt.wantText)
			}
		})
	}
}

func TestDeleteWordBackward(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		pos      int
		wantText string
		wantPos  int
	}{
		{"delete word from end", "hello world", 11, "hello ", 6},
		{"delete word with trailing spaces", "hello  ", 7, "", 0},
		{"delete from middle of word", "hello world", 8, "hello rld", 6},
		{"delete at start (no-op)", "hello", 0, "hello", 0},
		{"delete single word", "hello", 5, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotPos := deleteWordBackward(tt.text, tt.pos)
			if gotText != tt.wantText {
				t.Errorf("deleteWordBackward() text = %q, want %q", gotText, tt.wantText)
			}
			if gotPos != tt.wantPos {
				t.Errorf("deleteWordBackward() pos = %d, want %d", gotPos, tt.wantPos)
			}
		})
	}
}

func TestRuneLen(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"ascii", "hello", 5},
		{"unicode", "hello世界", 7},
		{"emoji", "hello👋", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runeLen(tt.text); got != tt.want {
				t.Errorf("runeLen() = %d, want %d", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Git URL Parsing Tests
// ============================================================================

func TestIsGitURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantIsGit bool
		wantURL  string
	}{
		// GitHub patterns
		{"github https url", "https://github.com/user/repo", true, "https://github.com/user/repo.git"},
		{"github https url with .git", "https://github.com/user/repo.git", true, "https://github.com/user/repo.git"},
		{"github http url", "http://github.com/user/repo", true, "https://github.com/user/repo.git"},
		{"github.com without protocol", "github.com/user/repo", true, "https://github.com/user/repo.git"},
		{"github git@ ssh url", "git@github.com:user/repo.git", true, "https://github.com/user/repo.git"},
		{"gh: shorthand", "gh:user/repo", true, "https://github.com/user/repo.git"},
		{"github trailing slash", "https://github.com/user/repo/", true, "https://github.com/user/repo.git"},

		// GitLab patterns
		{"gitlab https url", "https://gitlab.com/user/repo", true, "https://gitlab.com/user/repo.git"},
		{"gitlab.com without protocol", "gitlab.com/user/repo", true, "https://gitlab.com/user/repo.git"},
		{"gitlab git@ ssh url", "git@gitlab.com:user/repo.git", true, "https://gitlab.com/user/repo.git"},
		{"gl: shorthand", "gl:user/repo", true, "https://gitlab.com/user/repo.git"},
		{"gitlab nested group", "https://gitlab.com/group/subgroup/repo", true, "https://gitlab.com/group/subgroup/repo.git"},

		// Bitbucket patterns
		{"bitbucket https url", "https://bitbucket.org/user/repo", true, "https://bitbucket.org/user/repo.git"},
		{"bitbucket.org without protocol", "bitbucket.org/user/repo", true, "https://bitbucket.org/user/repo.git"},
		{"bitbucket git@ ssh url", "git@bitbucket.org:user/repo.git", true, "https://bitbucket.org/user/repo.git"},
		{"bb: shorthand", "bb:user/repo", true, "https://bitbucket.org/user/repo.git"},

		// Generic git URLs (returned as-is)
		{"generic https .git", "https://git.example.com/user/repo.git", true, "https://git.example.com/user/repo.git"},
		{"generic git@ ssh", "git@git.example.com:user/repo.git", true, "git@git.example.com:user/repo.git"},
		{"generic ssh://", "ssh://git@git.example.com/user/repo.git", true, "ssh://git@git.example.com/user/repo.git"},

		// Non-git URLs
		{"random text", "hello world", false, ""},
		{"empty", "", false, ""},
		{"partial url", "github.com", false, ""},
		{"no repo", "github.com/user", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIsGit, gotURL := isGitURL(tt.input)
			if gotIsGit != tt.wantIsGit {
				t.Errorf("isGitURL() isGit = %v, want %v", gotIsGit, tt.wantIsGit)
			}
			if gotURL != tt.wantURL {
				t.Errorf("isGitURL() url = %q, want %q", gotURL, tt.wantURL)
			}
		})
	}
}

// TestIsGitHubURL tests the backwards compatibility wrapper
func TestIsGitHubURL(t *testing.T) {
	// Just verify it calls isGitURL correctly
	isGit, url := isGitHubURL("https://github.com/user/repo")
	if !isGit || url != "https://github.com/user/repo.git" {
		t.Errorf("isGitHubURL() backwards compat failed")
	}
}

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"https url", "https://github.com/user/my-repo.git", "my-repo"},
		{"without .git", "https://github.com/user/my-repo", "my-repo"},
		{"complex name", "https://github.com/user/my.complex-repo_name.git", "my.complex-repo_name"},
		{"short url", "repo.git", "repo"},
		{"just name", "repo", "repo"},
		{"empty returns default", "", "repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractRepoName(tt.url); got != tt.want {
				t.Errorf("extractRepoName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Path and Branch Sanitization Tests
// ============================================================================

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   string
	}{
		{"simple", "main", "main"},
		{"with slash", "feature/my-feature", "feature-my-feature"},
		{"multiple slashes", "user/feature/sub", "user-feature-sub"},
		{"with backslash", "feature\\test", "feature-test"},
		{"double dots", "feature..test", "feature-test"},
		{"already clean", "my-feature-branch", "my-feature-branch"},
		{"with spaces", "feature my branch", "feature-my-branch"},
		{"with colons", "feature:test:branch", "feature-test-branch"},
		{"mixed special chars", "feat/my branch:v2", "feat-my-branch-v2"},
		{"preserves dots and underscores", "feat_1.0", "feat_1.0"},
		{"leading slash", "/feature", "feature"},
		{"trailing slash", "feature/", "feature"},
		{"multiple consecutive dashes", "feature---branch", "feature-branch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeBranchName(tt.branch); got != tt.want {
				t.Errorf("sanitizeBranchName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizePath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{"empty path", "", "", false},
		{"absolute path", "/tmp/test", "/tmp/test", false},
		{"tilde expansion", "~/test", filepath.Join(home, "test"), false},
		{"just tilde", "~", home, false},
		{"relative path", "test/dir", "", false}, // Will be made absolute, can't predict
		{"clean dots", "/tmp/../tmp/test", "/tmp/test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("sanitizePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Git Repository Detection Tests
// ============================================================================

func TestIsGitRepository(t *testing.T) {
	// Create a temp directory structure for testing
	tempDir := t.TempDir()

	// Create a fake git repo
	gitRepoDir := filepath.Join(tempDir, "git-repo")
	os.MkdirAll(filepath.Join(gitRepoDir, ".git"), 0755)

	// Create a nested directory inside the git repo
	nestedDir := filepath.Join(gitRepoDir, "nested", "deep")
	os.MkdirAll(nestedDir, 0755)

	// Create a non-git directory
	nonGitDir := filepath.Join(tempDir, "non-git")
	os.MkdirAll(nonGitDir, 0755)

	tests := []struct {
		name     string
		path     string
		wantRepo string
		wantIs   bool
	}{
		{"git repo root", gitRepoDir, gitRepoDir, true},
		{"nested in git repo", nestedDir, gitRepoDir, true},
		{"non-git directory", nonGitDir, "", false},
		{"non-existent", filepath.Join(tempDir, "nonexistent"), "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRepo, gotIs := isGitRepository(tt.path)
			if gotIs != tt.wantIs {
				t.Errorf("isGitRepository() isRepo = %v, want %v", gotIs, tt.wantIs)
			}
			if tt.wantIs && gotRepo != tt.wantRepo {
				t.Errorf("isGitRepository() repo = %q, want %q", gotRepo, tt.wantRepo)
			}
		})
	}
}

// ============================================================================
// Search Input Validation Tests
// ============================================================================

func TestIsValidSearchInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"letters", "hello", true},
		{"numbers", "123", true},
		{"mixed", "hello123", true},
		{"with dash", "hello-world", true},
		{"with underscore", "hello_world", true},
		{"with dot", "hello.world", true},
		{"with space", "hello world", true},
		{"with colon", "user:repo", true},
		{"with slash", "user/repo", true},
		{"with at", "user@host", true},
		{"empty", "", false},
		{"special chars", "hello!", false},
		{"newline", "hello\nworld", false},
		{"tab", "hello\tworld", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidSearchInput(tt.input); got != tt.want {
				t.Errorf("isValidSearchInput(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ============================================================================
// Fuzzy Matching/Scoring Tests
// ============================================================================

func TestCalculateScore(t *testing.T) {
	// Create a model with empty search term first
	m := model{searchTerm: ""}

	// Test entries
	now := time.Now()
	recentEntry := tryEntry{
		Basename: "2024-01-15-project-alpha",
		CTime:    now.Add(-24 * time.Hour), // 1 day old
		MTime:    now.Add(-1 * time.Hour),  // accessed 1 hour ago
	}
	oldEntry := tryEntry{
		Basename: "2024-01-01-old-project",
		CTime:    now.Add(-30 * 24 * time.Hour), // 30 days old
		MTime:    now.Add(-7 * 24 * time.Hour),  // accessed 7 days ago
	}

	t.Run("empty query uses time-based scoring", func(t *testing.T) {
		recentScore := m.calculateScore(recentEntry)
		oldScore := m.calculateScore(oldEntry)

		if recentScore <= oldScore {
			t.Errorf("recent entry should score higher: recent=%f, old=%f", recentScore, oldScore)
		}
	})

	t.Run("query filters non-matching", func(t *testing.T) {
		m.searchTerm = "xyz"
		score := m.calculateScore(recentEntry)
		if score != 0 {
			t.Errorf("non-matching entry should score 0, got %f", score)
		}
	})

	t.Run("query matches substring", func(t *testing.T) {
		m.searchTerm = "proj"
		score := m.calculateScore(recentEntry)
		if score == 0 {
			t.Error("matching entry should score > 0")
		}
	})

	t.Run("case insensitive matching", func(t *testing.T) {
		m.searchTerm = "PROJ"
		score := m.calculateScore(recentEntry)
		if score == 0 {
			t.Error("case-insensitive matching should work")
		}
	})

	t.Run("date-prefixed entries get bonus", func(t *testing.T) {
		m.searchTerm = ""
		dated := tryEntry{Basename: "2024-01-15-project", CTime: now, MTime: now}
		undated := tryEntry{Basename: "project", CTime: now, MTime: now}

		datedScore := m.calculateScore(dated)
		undatedScore := m.calculateScore(undated)

		if datedScore <= undatedScore {
			t.Errorf("dated entry should score higher: dated=%f, undated=%f", datedScore, undatedScore)
		}
	})

	t.Run("consecutive chars bonus", func(t *testing.T) {
		m.searchTerm = "proj"
		consecutive := tryEntry{Basename: "project", CTime: now, MTime: now}
		spread := tryEntry{Basename: "p-r-o-j-e-c-t", CTime: now, MTime: now}

		consScore := m.calculateScore(consecutive)
		spreadScore := m.calculateScore(spread)

		if consScore <= spreadScore {
			t.Errorf("consecutive should score higher: consecutive=%f, spread=%f", consScore, spreadScore)
		}
	})

	t.Run("shorter strings preferred", func(t *testing.T) {
		m.searchTerm = "proj"
		short := tryEntry{Basename: "project", CTime: now, MTime: now}
		long := tryEntry{Basename: "project-with-long-suffix", CTime: now, MTime: now}

		shortScore := m.calculateScore(short)
		longScore := m.calculateScore(long)

		if shortScore <= longScore {
			t.Errorf("shorter should score higher: short=%f, long=%f", shortScore, longScore)
		}
	})
}

// ============================================================================
// Shell Detection Tests
// ============================================================================

func TestDetectUserShell(t *testing.T) {
	// Save original env
	origTryShell := os.Getenv("TRY_SHELL")
	origShell := os.Getenv("SHELL")
	defer func() {
		os.Setenv("TRY_SHELL", origTryShell)
		os.Setenv("SHELL", origShell)
	}()

	t.Run("TRY_SHELL takes priority", func(t *testing.T) {
		os.Setenv("TRY_SHELL", "/usr/bin/fish")
		os.Setenv("SHELL", "/bin/bash")
		got := detectUserShell(nil)
		if got != "fish" {
			t.Errorf("detectUserShell() = %q, want %q", got, "fish")
		}
	})

	t.Run("config shell as fallback", func(t *testing.T) {
		os.Unsetenv("TRY_SHELL")
		os.Setenv("SHELL", "/bin/bash")
		config := &Config{Shell: "/usr/bin/zsh"}
		got := detectUserShell(config)
		if got != "zsh" {
			t.Errorf("detectUserShell() = %q, want %q", got, "zsh")
		}
	})

	t.Run("SHELL env fallback", func(t *testing.T) {
		os.Unsetenv("TRY_SHELL")
		os.Setenv("SHELL", "/bin/bash")
		got := detectUserShell(nil)
		if got != "bash" {
			t.Errorf("detectUserShell() = %q, want %q", got, "bash")
		}
	})

	t.Run("default to bash", func(t *testing.T) {
		os.Unsetenv("TRY_SHELL")
		os.Unsetenv("SHELL")
		got := detectUserShell(nil)
		if got != "bash" {
			t.Errorf("detectUserShell() = %q, want %q", got, "bash")
		}
	})
}

// ============================================================================
// Wrapper Generation Tests
// ============================================================================

func TestGenerateBashZshWrapper(t *testing.T) {
	wrapper := generateBashZshWrapper("/usr/local/bin/try")

	// Check essential parts
	if !strings.Contains(wrapper, "/usr/local/bin/try") {
		t.Error("wrapper should contain the binary path")
	}
	if !strings.Contains(wrapper, "--select-only") {
		t.Error("wrapper should use --select-only flag")
	}
	if !strings.Contains(wrapper, "cd \"$dir\"") {
		t.Error("wrapper should cd to the selected directory")
	}
}

func TestGenerateFishWrapper(t *testing.T) {
	wrapper := generateFishWrapper("/usr/local/bin/try")

	// Check essential parts
	if !strings.Contains(wrapper, "/usr/local/bin/try") {
		t.Error("wrapper should contain the binary path")
	}
	if !strings.Contains(wrapper, "--select-only") {
		t.Error("wrapper should use --select-only flag")
	}
	if !strings.Contains(wrapper, "function try") {
		t.Error("wrapper should define a fish function")
	}
	if !strings.Contains(wrapper, "cd $dir") {
		t.Error("wrapper should cd to the selected directory")
	}
}

// ============================================================================
// Shell Escape Tests
// ============================================================================

func TestShellEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple path", "/usr/local/bin/try", "'/usr/local/bin/try'"},
		{"path with spaces", "/Users/me/My Programs/try", "'/Users/me/My Programs/try'"},
		{"path with double quotes", `/path/with"quotes/try`, `'/path/with"quotes/try'`},
		{"path with backslash", `/path\with\backslash`, `'/path\with\backslash'`},
		{"path with single quote", "/path/with'quote/try", `'/path/with'\''quote/try'`},
		{"command injection attempt $", "/tmp/try$(rm -rf ~)", "'/tmp/try$(rm -rf ~)'"},
		{"command injection attempt backtick", "/tmp/try`rm -rf ~`", "'/tmp/try`rm -rf ~`'"},
		{"empty path", "", "''"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellEscape(tt.input); got != tt.want {
				t.Errorf("shellEscape() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Rename Validation Tests
// ============================================================================

func TestValidateRenameName(t *testing.T) {
	tempDir := t.TempDir()

	// Create an existing directory for collision testing
	if err := os.MkdirAll(filepath.Join(tempDir, "existing-dir"), 0755); err != nil {
		t.Fatalf("failed to create existing-dir: %v", err)
	}

	tests := []struct {
		name          string
		renameName    string
		originalName  string
		basePath      string
		wantError     bool
		errorContains string
	}{
		{"valid rename", "new-name", "old-name", tempDir, false, ""},
		{"same name is ok", "same-name", "same-name", tempDir, false, ""},
		{"empty name", "", "old-name", tempDir, true, "cannot be empty"},
		{"contains slash", "new/name", "old-name", tempDir, true, "cannot contain"},
		{"contains backslash", "new\\name", "old-name", tempDir, true, "cannot contain"},
		{"double dots in name is ok", "v1..v2", "old-name", tempDir, false, ""}, // Not traversal since / is blocked
		{"exact dot-dot blocked", "..", "old-name", tempDir, true, "Invalid name"},
		{"exact single dot blocked", ".", "old-name", tempDir, true, "Invalid name"},
		{"path traversal attempt", "../escape", "old-name", tempDir, true, "cannot contain"},
		{"collision with existing", "existing-dir", "old-name", tempDir, true, "already exists"},
		{"null byte blocked", "bad\x00name", "old-name", tempDir, true, "Invalid name"},
		{"absolute path blocked", "/etc/passwd", "old-name", tempDir, true, "absolute path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRenameName(tt.renameName, tt.originalName, tt.basePath)
			if tt.wantError && err == "" {
				t.Errorf("validateRenameName() expected error containing %q, got none", tt.errorContains)
			}
			if !tt.wantError && err != "" {
				t.Errorf("validateRenameName() unexpected error: %q", err)
			}
			if tt.wantError && tt.errorContains != "" && !strings.Contains(err, tt.errorContains) {
				t.Errorf("validateRenameName() error = %q, want containing %q", err, tt.errorContains)
			}
		})
	}
}

