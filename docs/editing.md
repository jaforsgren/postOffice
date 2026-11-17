# Editing requests

Select a request and press `e`. Edit the name, method, URL, path parameters, headers, or body. See [the editor keys](commands-and-keys.md#request-editor).

Path parameters such as `/users/:id` have dedicated fields. Values persist during the session, and resolved values are shown in the editor.

## Autocomplete

Type `{{` for variable autocomplete. Variables whose name starts with what you typed come first, then those containing it (`{{claim` suggests `{{claim_id}}` and `{{my_claim_type}}`). Each suggestion shows its resolved value, its scope (environment, collection or folder) and any definitions it shadows, e.g. `Environment: Local · shadows Collection: SAPI = abc`.

Outside `{{ }}`, a word of three or more characters suggests values of matching JSON fields seen in request history (responses received and requests sent), saved responses and request bodies saved in the collection, newest first. Matching ignores case, `_` and `-`, so `persona_` suggests the values of `persona_id` and `personaId`, each with where it was seen.

Press `tab` to insert the selected suggestion (a variable replaces the `{{…`, a value replaces the typed word) and `tab` again to replace it with the next one.

## Variables

Use `V` to edit a request's variables at their source.

A variable can hold a list of choices (`"choices": ["01", "02"]` next to its `value` in the environment or collection file). Its `value` is the chosen one. In the `V` panel, `enter` opens a dropdown to pick another choice, `a` adds a choice (turning a plain variable into a choice list) `d` deletes one and `c` adds an optional note saying what the choice means (`"choiceNotes": {"02": "admin persona"}`; an empty note removes it). Reference a specific choice with `{{persona_id[1]}}` (zero-based).

## Saving and undo

Unsaved edits are marked with `*` on the request, every folder above it and its collection (and on edited environments).

Use `:changes` to review unsaved edits. Press `d` to discard one or `ctrl+d` to discard all.

Press `u` to undo the last saved change (delete, duplicate, add, edit, rename, editor edits and variable values) and `ctrl+r` to redo it, in the collections, requests and environments views. Undo restores the file, so save or discard unsaved edits first.
