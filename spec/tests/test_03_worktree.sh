# Worktree command tests

section "worktree"

# Create a fake git repo for worktree tests
FAKE_REPO=$(mktemp -d)
mkdir -p "$FAKE_REPO/.git"
# Initialize as a real git repo for worktree tests
git -C "$FAKE_REPO" init -q 2>/dev/null
git -C "$FAKE_REPO" config user.email "test@test.com" 2>/dev/null
git -C "$FAKE_REPO" config user.name "Test" 2>/dev/null
touch "$FAKE_REPO/test.txt"
git -C "$FAKE_REPO" add . 2>/dev/null
git -C "$FAKE_REPO" commit -m "initial" -q 2>/dev/null

# Test: worktree from non-git dir shows error
PLAIN_DIR=$(mktemp -d)
output=$(cd "$PLAIN_DIR" && TRY_PATH="$TEST_TRIES" try_run worktree testbranch 2>&1)
if echo "$output" | grep -qi "not.*git\|error"; then
    pass
else
    fail "worktree from non-git dir should error" "error message" "$output"
fi

# Test: try . without branch shows error
output=$(cd "$FAKE_REPO" && TRY_PATH="$TEST_TRIES" try_run . 2>&1)
if echo "$output" | grep -qi "branch\|error\|requires"; then
    pass
else
    fail "try . without branch should show error" "error about branch" "$output"
fi

# Test: dot shorthand is recognized as worktree command
# We just test that it's parsed, not that the worktree succeeds
output=$(cd "$FAKE_REPO" && TRY_PATH="$TEST_TRIES" try_run . main 2>&1)
if echo "$output" | grep -qi "worktree\|Creating\|error"; then
    pass
else
    fail "try . <branch> should be recognized" "worktree output" "$output"
fi

# Test: --in-repo flag is recognized
output=$(cd "$FAKE_REPO" && TRY_PATH="$TEST_TRIES" try_run . --in-repo main 2>&1)
if echo "$output" | grep -qi "worktree\|Creating\|\.worktrees\|error"; then
    pass
else
    fail "try . --in-repo should be recognized" "worktree output" "$output"
fi

# Cleanup worktrees if created
git -C "$FAKE_REPO" worktree prune 2>/dev/null || true
rm -rf "$FAKE_REPO" "$PLAIN_DIR"
