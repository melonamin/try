# Clone command tests

section "clone"

# Test: --clone requires URL argument
output=$(try_run --clone 2>&1)
if echo "$output" | grep -qi "requires\|error\|url"; then
    pass
else
    fail "--clone without URL should error" "error message" "$output"
fi

# Test: -c requires URL argument
output=$(try_run -c 2>&1)
if echo "$output" | grep -qi "requires\|error\|url"; then
    pass
else
    fail "-c without URL should error" "error message" "$output"
fi

# Note: We don't test actual cloning as it requires network access
# The unit tests cover URL parsing
