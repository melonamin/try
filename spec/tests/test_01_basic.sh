# Basic compliance tests: --help, --version

section "basic"

# Test --help
output=$(try_run --help 2>&1)
if echo "$output" | grep -q "Quick Experiment Directories"; then
    pass
else
    fail "--help missing expected text" "contains 'Quick Experiment Directories'" "$output"
fi

# Test -h
output=$(try_run -h 2>&1)
if echo "$output" | grep -q "Quick Experiment Directories"; then
    pass
else
    fail "-h missing expected text" "contains 'Quick Experiment Directories'" "$output"
fi

# Test --version
output=$(try_run --version 2>&1)
if echo "$output" | grep -qE "^try version [0-9]+\.[0-9]+"; then
    pass
else
    fail "--version format incorrect" "try version X.Y.Z" "$output"
fi

# Test -v
output=$(try_run -v 2>&1)
if echo "$output" | grep -qE "^try version [0-9]+\.[0-9]+"; then
    pass
else
    fail "-v format incorrect" "try version X.Y.Z" "$output"
fi

# Test help shows worktree command
output=$(try_run --help 2>&1)
if echo "$output" | grep -q "worktree"; then
    pass
else
    fail "--help should mention worktree" "worktree in help" "$output"
fi

# Test help shows init command
if echo "$output" | grep -q "try init"; then
    pass
else
    fail "--help should mention init" "try init in help" "$output"
fi

# Test help shows rename shortcut
if echo "$output" | grep -q "Ctrl+R"; then
    pass
else
    fail "--help should mention Ctrl+R rename" "Ctrl+R in help" "$output"
fi

# Test help shows text editing shortcuts
if echo "$output" | grep -q "Ctrl+A"; then
    pass
else
    fail "--help should mention text editing shortcuts" "Ctrl+A in help" "$output"
fi
