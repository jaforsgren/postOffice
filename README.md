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
- `j` - Show JSON view of selected item
- `/` - Search items
- `e` - Edit selected request or collection
- `v` - Manage variables/environments
- `esc/h/backspace` - Go back/up
- `q` or `ctrl+c` - Quit

**Viewport Scrolling (Response/Info/JSON views):**
- `j/k` or `↓/↑` - Scroll down/up one line
- `d` or `Page Down` - Scroll down half page
- `u` or `Page Up` - Scroll up half page
- `g` or `Home` - Jump to top
- `G` or `End` - Jump to bottom
- `esc/h/backspace` - Close view

**Command Mode:**

Press `:` to enter command mode:

- `:load <path>` or `:l <path>` - Load a Postman collection
- `:loadenv <path>` or `:le <path>` - Load an environment file
- `:collections` or `:c` - Switch to collections view
- `:requests` or `:r` - Switch to requests view
- `:environments` or `:env` - Switch to environments view
- `:variables` or `:var` - Show all variables
- `:info` or `:i` - Display item info
- `:edit` - Edit selected request
- `:w` - Save changes to file
- `:wq` - Save changes and quit
- `:changes` or `:ch` - Show unsaved changes
- `:help` or `:h` - Show help
- `:quit` or `:q` - Exit

**Search Mode:**

Press `/` to enter search mode:
- Type to filter items
- Press `enter` to confirm search
- Press `esc` to cancel search
- Results update as you type

### File Paths

Collections and environments are automatically saved to `~/.postoffice_collections.json` and restored on next startup.

When loading files, `~/` is expanded to your home directory:

```
:load ~/postman/my-collection.json
```

## Request Execution

1. Navigate to a request using `j/k`
2. Press `enter` to execute (or `ctrl+r`)
3. View the response in the viewport
4. Use `j/k` to scroll the response (or `d/u` for half-page scrolling)
5. Press `esc` to close

## Editing Requests

1. Navigate to a request and press `e` or use `:edit`
2. Use `j/k` to navigate between fields (Name, Method, URL, Headers, Body)
3. Press `enter` to edit a field
4. **For single-line fields (Name, Method, URL):**
   - Type your changes
   - Press `enter` to save
   - Press `esc` to cancel
5. **For multi-line fields (Headers, Body):**
   - Type your changes
   - Press `enter` for newlines
   - Press `ctrl+s` to save
   - Press `esc` to cancel
6. Press `esc` to exit edit mode (changes saved to memory)
7. Use `:w` to write changes to file
8. Use `:wq` to write changes and quit

**Managing Unsaved Changes:**

- `:changes` - View all unsaved changes
- In changes view:
  - `d` - Discard selected change
  - `ctrl+d` - Discard all changes
  - `esc` - Close changes view

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
