# PostOffice

A terminal UI for browsing and executing Postman collections with vim-style navigation.

## Features

- Browse Postman collections in a terminal interface
- Execute HTTP requests and view responses
- Vim-style keyboard navigation
- Persistent session state
- Environment variable support
- Request and collection editing

## Installation

### Build from source

```bash
go build -o postOffice
```

## Usage

### Basic Usage

```bash
# Run the application
./postOffice

# Run with file operation logging (for debugging)
./postOffice --log debug.log
```

### Navigation

**Normal Mode:**
- `j/k` or `↓/↑` - Navigate items
- `enter` - Select item (load collection, open folder, execute request)
- `i` - Show info for selected item
- `e` - Edit selected request or collection
- `v` - Manage variables/environments
- `esc/h/backspace` - Go back/up
- `q` or `ctrl+c` - Quit

**Command Mode:**

Press `:` to enter command mode:

- `:load <path>` or `:l <path>` - Load a Postman collection
- `:collections` or `:c` - Switch to collections view
- `:requests` or `:r` - Switch to requests view
- `:info` or `:i` - Display item info
- `:help` or `:h` - Show help
- `:quit` or `:q` - Exit

### File Paths

Collections and environments are automatically saved to `~/.postoffice_collections.json` and restored on next startup.

When loading files, `~/` is expanded to your home directory:

```
:load ~/postman/my-collection.json
```

## Request Execution

1. Navigate to a request using `j/k`
2. Press `enter` to execute
3. View the response in the popup
4. Use `j/k` to scroll the response
5. Press `esc` to close

## Environment Variables

1. Press `v` to open variable management
2. Load an environment file with `:load <env-path>`
3. Edit variable values directly in the UI
4. Variables are automatically applied to requests

## Debugging

Enable file operation logging to troubleshoot issues:

```bash
./postOffice --log debug.log
```

The log file will contain:
- `[FILE_OPEN]` - File read attempts
- `[FILE_WRITE]` - File write attempts
- `[ERROR]` - Any errors encountered

## Development

```bash
# Format code
go fmt ./...

# Vet code
go vet ./...

# Run tests
go test ./...
```

## License

MIT
