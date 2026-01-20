# Select-only mode tests

section "select-only"

# Note: Select-only mode requires a TTY for the TUI
# We can only test that the flag is recognized

# Test: --select-only is documented
output=$(try_run --help 2>&1)
if echo "$output" | grep -qE -- "--select-only|-s"; then
    pass
else
    fail "--select-only should be in help" "--select-only or -s" "$output"
fi

# Test: -s is documented
if echo "$output" | grep -q -- "-s"; then
    pass
else
    fail "-s should be in help" "-s flag" "$output"
fi
