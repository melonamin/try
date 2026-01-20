# Init command tests

section "init"

# Test: init generates shell wrapper
output=$(try_run init 2>&1)
if echo "$output" | grep -q "Detected shell"; then
    pass
else
    fail "init should detect shell" "Detected shell" "$output"
fi

# Test: init generates function definition (bash: "try()" or fish: "function try")
if echo "$output" | grep -qE "try\(\)|function try"; then
    pass
else
    fail "init should generate try function" "try() or function try" "$output"
fi

# Test: init uses --select-only
if echo "$output" | grep -q -- "--select-only"; then
    pass
else
    fail "init wrapper should use --select-only" "--select-only" "$output"
fi

# Test: init shows config file suggestion
if echo "$output" | grep -q "Suggested config file"; then
    pass
else
    fail "init should suggest config file" "Suggested config file" "$output"
fi

# Test: init with custom path
output=$(try_run init /custom/path/to/try 2>&1)
if echo "$output" | grep -q "/custom/path/to/try"; then
    pass
else
    fail "init should use custom path" "/custom/path/to/try" "$output"
fi
