<img width="768" height="512" alt="image" src="https://github.com/user-attachments/assets/86036472-c6d3-4774-8c72-2d86bb920a15" />

# try - fresh directories for every vibe

> A Go reimplementation of [tobi/try](https://github.com/tobi/try) with GitHub cloning, deletion support, and better performance

Ever find yourself with 50 directories named `test`, `test2`, `new-test`, `actually-working-test`, scattered across your filesystem? Or worse, just coding in `/tmp` and losing everything?

**try** is here for your beautifully chaotic mind.

## What it does

Instantly navigate through all your experiment directories with:
- **Fuzzy search** that just works
- **Smart sorting** - recently used stuff bubbles to the top
- **Auto-dating** - creates directories like `2025-08-17-redis-experiment`
- **Zero config** - single binary, no dependencies

## Installation

### Using Go
```bash
go install github.com/melonamin/try@latest
```

### Build from Source
```bash
git clone https://github.com/melonamin/try.git
cd try
go build -o ~/.local/bin/try
```


## The Problem

You're learning Redis. You create `/tmp/redis-test`. Then `~/Desktop/redis-actually`. Then `~/projects/testing-redis-again`. Three weeks later you can't find that brilliant connection pooling solution you wrote at 2am.

## The Solution

All your experiments in one place, with instant fuzzy search:

```bash
$ try pool
→ 2025-08-14-redis-connection-pool    2h, 18.5
  2025-08-03-thread-pool              3d, 12.1
  2025-07-22-db-pooling               2w, 8.3
  + Create new: pool
```

Type, arrow down, enter. You're there.

## What's New in This Fork

### 📦 GitHub Repository Cloning
- Clone repos directly: `try --clone https://github.com/user/repo`
- Auto-detect GitHub URLs in search
- Creates dated folders like `2025-01-21-repo-name`

### 🗑️ Directory Deletion
- Press `Ctrl+D` to delete directories
- Safe two-step confirmation process
- Visual warnings to prevent accidents

### ⚡ Performance Improvements
- Pre-compiled regex patterns
- Faster startup and search
- Timeout protection for git operations

## Original Features

### 🎯 Smart Fuzzy Search
Not just substring matching - it's smart:
- `rds` matches `redis-server`
- `connpool` matches `connection-pool`
- Recent stuff scores higher
- Shorter names win on equal matches

### ⏰ Time-Aware
- Shows how long ago you touched each project
- Recently accessed directories float to the top
- Perfect for "what was I working on yesterday?"

### 🎨 Pretty TUI
- Clean, minimal interface
- Highlights matches as you type
- Shows scores so you know why things are ranked
- Dark mode by default (because obviously)

### 📁 Organized Chaos
- Everything lives in `~/src/tries` (configurable via `TRY_PATH`)
- Auto-prefixes with dates: `2025-08-17-your-idea`
- Skip the date prompt if you already typed a name

## Usage

```bash
try                                      # Browse all experiments
try redis                                # Jump to redis experiment or create new
try new api                              # Start with "2025-01-21-new-api"
try github.com/user/repo                 # Shows clone option in TUI
try --clone https://github.com/user/repo # Clone directly without TUI
try --help                               # See all options
```

### Keyboard Shortcuts

- `↑/↓` - Navigate entries
- `Ctrl+j/k` - Navigate (vim-style)
- `Enter` - Select directory or create new
- `Ctrl+N` - Quick create new experiment
- `Ctrl+D` - Delete selected directory
- `Ctrl+U` - Clear search
- `ESC/q` - Cancel and exit
- Just type to filter

## Configuration

Set `TRY_PATH` to change where experiments are stored:

```bash
export TRY_PATH=~/code/sketches
```

Default: `~/src/tries`

## Comparison with Original

| Feature | Original (Ruby) | This Fork (Go) |
|---------|----------------|----------------|
| **Language** | Ruby | Go |
| **Installation** | Ruby required | Single binary |
| **GitHub Cloning** | ❌ | ✅ Built-in |
| **Delete Directories** | ❌ | ✅ With confirmation |
| **Paste Support** | ✅ | ✅ Full URL paste |
| **Performance** | Good | Excellent |
| **Binary Size** | ~20KB + Ruby | ~4-5MB standalone |
| **Cross-platform** | Ruby dependent | Native binaries |
| **Shell Integration** | ✅ Via eval | ❌ Spawns subshell |
| **Dependencies** | Ruby runtime | None |

## Acknowledgements

This project is a fork of [tobi/try](https://github.com/tobi/try), originally created by Tobi Lütke. The original Ruby implementation provided the excellent foundation and user experience that this Go version builds upon.

Key additions in this fork:
- GitHub repository cloning functionality
- Directory deletion with safety confirmation
- Performance optimizations for large directory sets
- Enhanced keyboard shortcuts

## The Philosophy

Your brain doesn't work in neat folders. You have ideas, you try things, you context-switch like a caffeinated squirrel. This tool embraces that.

Every experiment gets a home. Every home is instantly findable. Your 2am coding sessions are no longer lost to the void.

## FAQ

**Q: Why not just use `cd` and `ls`?**
A: Because you have 200 directories and can't remember if you called it `test-redis`, `redis-test`, or `new-redis-thing`.

**Q: Why not use `fzf`?**
A: fzf is great for files. This is specifically for project directories, with time-awareness and auto-creation built in.

**Q: Can I use this for real projects?**
A: You can, but it's designed for experiments. Real projects deserve real names in real locations.

**Q: What if I have thousands of experiments?**
A: First, welcome to the club. Second, it handles it fine - the scoring algorithm ensures relevant stuff stays on top.

## Contributing

Contributions are welcome! The codebase is a single Go file (`main.go`) using the Bubble Tea TUI framework. Feel free to submit issues or pull requests.

## License

MIT - Do whatever you want with it.
