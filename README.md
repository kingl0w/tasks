# tasks

A small command-line task manager written in Go, backed by SQLite.

```
$ tasks add "Write the README" -priority high -due 2026-09-15
added task 1

$ tasks add "Buy milk"
added task 2

$ tasks list -sort due
ID     PRI     DUE         TITLE
1      high    2026-09-15  Write the README
2      medium              Buy milk

$ tasks done 2
completed task 2

$ tasks export -o tasks.csv
exported 2 tasks to tasks.csv
```

## Install

Requires Go 1.25 or newer. No C compiler needed; the SQLite driver is pure Go.

```
go install github.com/kingl0w/tasks@latest
```

Or clone and `go build`, which leaves a `tasks` binary in the current directory.

## Usage

```
tasks add <title>      add a task            -priority low|medium|high  -due YYYY-MM-DD
tasks list             show open tasks       -all  -done  -priority P  -sort created|due|priority
tasks update <id>      change a task         -title T  -priority P  -due D  (use -due "" to clear)
tasks done <id>        mark a task complete
tasks undo <id>        reopen a task
tasks rm <id>          delete a task
tasks export           write tasks as CSV    -o FILE (default stdout)
```

In `list` output, `!` marks an overdue task and `x` marks a completed one.

The database is created on first use at `~/.tasks.db`. Set `TASKS_DB` to put it somewhere else,
which is also handy for keeping separate lists:

```
TASKS_DB=work.db tasks add "Ship the release"
```

## Approach

I picked the CLI task manager because it is the option where a small, readable codebase can
cover everything asked for without a framework getting in the way. The whole thing is three
source files:

- `store.go` is the data layer. It owns the `Task` type, the schema, validation, and every SQL
  statement. Nothing else in the program touches the database.
- `main.go` is the command-line layer. It parses arguments, calls the store, and prints results.
- `store_test.go` exercises the store against an in-memory database, so the tests are fast and
  leave nothing on disk.

Some choices worth explaining:

**Standard library `flag` instead of Cobra.** Seven subcommands with two or three flags each
don't justify a dependency. Each command gets its own `FlagSet`, which keeps the flag
definitions next to the code that uses them. The one wrinkle is that `flag` stops at the first
non-flag argument, so the title or id always comes first and flags follow it: `tasks add "Title"
-due X`. A three-line helper peels that first argument off before parsing.

**Pure-Go SQLite (`modernc.org/sqlite`).** The usual `mattn/go-sqlite3` needs CGO and a C
toolchain, which makes cross-compiling and `go install` more annoying than they should be for a
tool this size. The modernc driver is a drop-in replacement with none of that.

**Validation in the store, and again in the schema.** Titles, priorities and dates are checked in
Go so the user gets a clear error message, and the table also has `CHECK` constraints so bad data
can't sneak in through some future code path. Due dates are stored as `YYYY-MM-DD` text, which
sorts correctly as a string and is what the user typed, so there is no timezone ambiguity.

**`update` only touches the flags you pass.** The command inspects which flags were explicitly
set (via `flag.Visit`) and passes `nil` for the rest, so `tasks update 3 -priority high` leaves
the title and due date alone, while `-due ""` clears the due date on purpose.

**Safe to run concurrently.** SQLite locks the file during a write, so two `tasks` processes
started at the same moment (a cron job and you, say) would normally fail with "database is
locked". The connection sets a 5 second busy timeout and WAL journaling, and I checked it by
firing 30 `add` commands in parallel: all 30 land. At 200k rows, listing everything takes about
a quarter of a second, which is far past what a task list needs.

**No TUI.** The brief lists it as a bonus. I left it out because the plain CLI composes better
with shell tools (`tasks export | column -s, -t`, cron jobs, and so on), and adding a screen
library would triple the dependency footprint for a feature I would not use myself. It would slot
in as a fourth file that reads from the same `Store` if wanted.

## Tests

```
go test ./...
```

The tests cover validation, filtering and sorting, the update/done/delete lifecycle, and the
overdue check.
