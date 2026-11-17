# Commands and keys

Press `?` (or `:h`) in any view for the keys that work there.

## Commands

Press `:` to enter command mode. `esc` leaves it, `enter` runs the command.

| Command | Action |
|---|---|
| `:l <path>` | Load a collection |
| `:le <path>` | Load an environment |
| `:c` | Collections |
| `:r` | Requests |
| `:env` | Environments |
| `:var` | Variables |
| `:cv` | Edit the collection's own variables |
| `:edit` | Edit the selected request |
| `:wf` | Workflows |
| `:wf new <id>` | Create a workflow |
| `:wf run <id>` | Run a workflow |
| `:ro` | [Run options](workflows.md#run-options) |
| `:changes` | Unsaved changes |
| `:session` | Restore the last session |
| `:hist` | Request history |
| `:lis` | [Listeners](servicebus.md#watching): messages seen by watched PEEK requests |
| `:deps` | Dependency graph of the selected request |
| `:t` | Timing breakdown (response view) |
| `:w` | Save |
| `:wq` | Save and quit |
| `:q` | Quit |
| `:h` | Help |

Paths support `~/` expansion, e.g. `:load ~/postman/api.json`. Loaded collections and environments are restored on startup.

## Every view

| Key | Action |
|---|---|
| `ctrl+e` | Run: send the request, run the folder, resend, run the workflow |
| `1`–`4` | Collections / Requests / Environments / Variables |
| `T` | Toggle the top bar |
| `?` | Help |
| `q` | Back; quits from Collections |
| `ctrl+c` | Quit |

## Lists

| Key | Action |
|---|---|
| `j` / `k` | Move up/down |
| `g` / `G` | First/last item |
| `enter` | Open or execute |
| `h` / `esc` | Go back |
| `/` | Search (collections, requests, environments, variables, workflows) |
| `i` | Info (also on a collection) |
| `tab` | Toggle split view |
| `ctrl+o` / `ctrl+]` | Jump back/forward (previous folder or collection, also from other lists) |
| `O` | Reveal the collection/environment file (Finder/Explorer) |
| `r` | Refresh |
| `ctrl+n` | Next environment |
| `ctrl+y` | Request history |

## Requests

| Key | Action |
|---|---|
| `e` / `E` | Edit request / all scripts in `$EDITOR` (one tab each) |
| `d` / `D` | Delete / duplicate |
| `f` | Toggle favorite |
| `i` | Request info + response |
| `J` | Raw JSON |
| `H` | Saved responses |
| `V` | Edit used variables (on a folder: the folder's own variables) |
| `ctrl+e` | Execute request / run folder |
| `P` | Run stale prerequisites, then the request |
| `L` | Edit request dependencies |
| `M` | Dependency graph |
| `ctrl+x` | Cancel the request being sent; in the workflow views, stop the running workflow |
| `w` | Watch the selected [PEEK request](servicebus.md#peeking) in the background, or every one in the selected folder (again: stop) |
| `W` | Listeners view |
| `z` / `Z` | Expand/collapse the folder in place / all folders |
| `shift+↑` / `shift+↓` | Jump to the previous/next folder |
| `.` | Repeat the last action (execute, delete, duplicate, favorite, expand) |
| `u` / `ctrl+r` | Undo / redo the last saved change |

## Responses

| Key | Action |
|---|---|
| `j` / `k` | Scroll |
| `g` / `G` | Top/bottom |
| `d` / `u` | Half-page scroll |
| `ctrl+e` | Resend request |
| `t` | Timing breakdown |
| `s` | Save response |
| `ya` | Copy request + response |
| `yb` | Copy response body |
| `yr` | Copy as curl/grpcurl |
| `yy` | Copy resolved URL |
| `o` | Open Markdown report |

## Request editor

| Key | Action |
|---|---|
| `j` / `k` | Select field |
| `enter` | Edit / save a single-line field |
| `ctrl+s` | Save a multi-line field |
| `tab` | Insert the selected suggestion, again for the next one |
| `V` | Edit the request's variables |
| `ctrl+g` | Open in `$EDITOR` |
| `esc` | Cancel / exit |
| `:w` | Write changes to file |

The gRPC editor adds `enter` to toggle TLS and `ctrl+f` to browse server reflection.

## Dependency graph

| Key | Action |
|---|---|
| `j` / `k` | Select a request |
| `enter` / `r` / `P` / `ctrl+e` | Run the selected request after its stale prerequisites |
| `l` / `→` | Jump to the request in the requests view |
| `esc` / `q` | Back |

## Request history

| Key | Action |
|---|---|
| `j` / `k` | Navigate entries (newest first) |
| `enter` | Replay the entry |
| `=` | Diff the entry against the current response |
| `esc` / `q` | Close the diff, then the history |

## Listeners

| Key | Action |
|---|---|
| `j` / `k` | Navigate messages (newest enqueued first) |
| `enter` | Show the message's properties and body |
| `n` / `N` | Next / previous message (message view) |
| `ctrl+e` | Send the message again to its queue or topic |
| `S` | Save the message as an `ASB` request next to its PEEK request |
| `y` | Copy the message body |
| `o` | Open the message (message view) or every listed message as JSON in `$EDITOR` |
| `/` | Search subjects, properties and bodies (`esc` clears) |
| `f` | Show one listener's messages; again for the next, then all |
| `p` | Pause: hold new messages back until `p` again |
| `c` | Clear the messages seen so far |
| `ctrl+x` | Stop every listener |
| `esc` / `q` | Close the message, then the listeners |

## Workflows

| Key | Action |
|---|---|
| `a` | Add step |
| `e` / `E` | Edit request/scripts |
| `<` / `>` | Run up to / from the step |
| `x` | Run the step |
| `R`, `ctrl+e` | Run all |
| `.` | Repeat the last partial run |
| `r` | Reload workflows from disk |
| `enter` | Step response |
| `tab` | Step panel (`[` / `]` scroll it) |
| `i` | Step info |
| `ctrl+d` | Delete step |
| `o` | Open the run report |
