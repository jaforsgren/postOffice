# Headless CLI

Run requests, folders, collections, and workflows without the TUI.

## Execute

```bash
# Single request
postOffice run collection.json "Users/Create User" -e dev.json

# Folder
postOffice run collection.json --folder smoke -e staging.json

# Collection
postOffice run collection.json -e staging.json

# Workflow
postOffice workflow run workflows/check.yaml -e staging.json
```

Collections, environments and workflows can also be given by name instead of file path. Names are matched against loaded files (see `ls`), ignoring case: a collection or environment by its own name or file name (`SAPI` for `SAPI.postman_collection.json`), and a workflow by its ID, name or file name.

```bash
postOffice run SAPI -e "Big tusks"
postOffice workflow run health-check -e "Big tusks"
```

Request paths use `/` between folders. Escape literal slashes in names with `\/` and quote the path.

Headless collection runs support `pm.execution.setNextRequest()` for jumps and early termination.

The text report streams each step as it finishes, with ✓ passed in green, ✗ failed in red and − skipped in yellow. Colors are only used on a terminal (and never with `NO_COLOR`).

## Selection

Select a collection, environment and workflow once, and headless commands default to them:

```bash
postOffice -c SAPI -e "Big tusks"     # same as: postOffice use -c SAPI -e "Big tusks"
postOffice run                        # runs SAPI with Big tusks
postOffice run "Users/Get User"       # one request in SAPI
postOffice -w health-check
postOffice workflow run               # runs health-check with Big tusks

postOffice use                        # show the selection
postOffice use -e ""                  # unselect the environment
postOffice use --clear                # unselect everything
```

An explicit `-e` or collection argument still wins. The selection is stored under `cli` in `~/.postoffice_session.json`, and `ls` marks it with `*`.

## CI and reports

```bash
postOffice run collection.json --folder smoke -e ci.json --output junit --report results.xml
```

| Option | Description |
|---|---|
| `--output text\|json\|junit` | Report format |
| `--report <file>` | Write the report to a file |
| `--iterations <n>` | Run the collection n times |
| `--data <file.csv\|json>` | Data file, one iteration per row |
| `--bail` | Stop at the first failure |
| `--persist` | Write script-modified variables back to files (not OpenAPI files) |
| `-e, --environment <file>` | Environment |

JSON includes request results, tests, errors, timings, and console output. JUnit produces test suites and test cases.

| Exit code | Meaning |
|---|---|
| `0` | All passed |
| `1` | Request, test, or script failure |
| `2` | Invalid arguments or input |

## Listing

Inspect loaded collections and environments:

```bash
postOffice ls
postOffice ls var
postOffice ls request

postOffice ls var -c collection.json -e dev.json
postOffice ls r -c collection.json
```

## Merging

Merge collections or environments without opening the TUI:

```bash
postOffice merge a.json b.json --forceMerge                          # second file wins
postOffice merge a.json b.json --keep-original                       # first file wins
postOffice merge a.json b.json --forceMerge -o merged.json           # write to a new file
postOffice merge a.json b.json --forceMerge --group-by-collection    # keep collections grouped
```

- Both inputs must be the same type.
- Folders merge recursively.
- Conflicts follow the selected strategy.
- Unique items are preserved.
- The first file is overwritten unless `-o` is used.
- Grouping keeps each collection under its own folder and avoids double-wrapping existing groups.

`-m` is an alias for `merge`.

## Aliases

Save frequently used commands:

```bash
postOffice alias add smoke run collection.json --folder smoke -e ci.json

postOffice alias list
postOffice alias run smoke
postOffice alias remove smoke
```

Aliases support `run`, `workflow`, `ls`, and `merge`. Paths resolve from the current working directory. Exit codes are preserved.
