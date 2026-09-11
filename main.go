package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

const usage = `usage: tasks <command> [flags]

commands:
  add <title>      add a task            -priority low|medium|high  -due YYYY-MM-DD
  list             show open tasks       -all  -done  -priority P  -sort created|due|priority
  update <id>      change a task         -title T  -priority P  -due D  (use -due "" to clear)
  done <id>        mark a task complete
  undo <id>        reopen a task
  rm <id>          delete a task
  export           write tasks as CSV    -o FILE (default stdout)  -all

The database lives at $TASKS_DB, or ~/.tasks.db if unset.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Print(usage)
		return nil
	}

	store, err := Open(dbPath())
	if err != nil {
		return err
	}
	defer store.Close()

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "add":
		return cmdAdd(store, rest)
	case "list":
		return cmdList(store, rest)
	case "update":
		return cmdUpdate(store, rest)
	case "done", "undo", "rm":
		return cmdOne(store, cmd, rest)
	case "export":
		return cmdExport(store, rest)
	}
	return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
}

func dbPath() string {
	if p := os.Getenv("TASKS_DB"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "tasks.db"
	}
	return filepath.Join(home, ".tasks.db")
}

func cmdAdd(s *Store, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	priority := fs.String("priority", "medium", "low, medium or high")
	due := fs.String("due", "", "due date, YYYY-MM-DD")
	title, args, err := splitFirst(args, "title")
	if err != nil {
		return err
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := s.Add(title, *priority, *due)
	if err != nil {
		return err
	}
	fmt.Printf("added task %d\n", id)
	return nil
}

func cmdList(s *Store, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	var f Filter
	fs.BoolVar(&f.All, "all", false, "include completed tasks")
	fs.BoolVar(&f.DoneOnly, "done", false, "only completed tasks")
	fs.StringVar(&f.Priority, "priority", "", "only this priority")
	fs.StringVar(&f.Sort, "sort", "created", "created, due or priority")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tasks, err := s.List(f)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		fmt.Println("no tasks")
		return nil
	}
	printTable(tasks)
	return nil
}

func cmdUpdate(s *Store, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	title := fs.String("title", "", "new title")
	priority := fs.String("priority", "", "new priority")
	due := fs.String("due", "", "new due date, or empty to clear")
	idStr, args, err := splitFirst(args, "id")
	if err != nil {
		return err
	}
	id, err := parseID(idStr)
	if err != nil {
		return err
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Only fields the user explicitly set; the rest stay nil.
	var t, p, d *string
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "title":
			t = title
		case "priority":
			p = priority
		case "due":
			d = due
		}
	})
	if err := s.Update(id, t, p, d); err != nil {
		return err
	}
	fmt.Printf("updated task %d\n", id)
	return nil
}

func cmdOne(s *Store, cmd string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: tasks %s <id>", cmd)
	}
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	var verb string
	switch cmd {
	case "done":
		err, verb = s.SetDone(id, true), "completed"
	case "undo":
		err, verb = s.SetDone(id, false), "reopened"
	case "rm":
		err, verb = s.Delete(id), "deleted"
	}
	if err != nil {
		return err
	}
	fmt.Printf("%s task %d\n", verb, id)
	return nil
}

func cmdExport(s *Store, args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	out := fs.String("o", "", "output file (default stdout)")
	all := fs.Bool("all", true, "include completed tasks")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tasks, err := s.List(Filter{All: *all})
	if err != nil {
		return err
	}

	var w io.Writer = os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	if err := writeCSV(w, tasks); err != nil {
		return err
	}
	if *out != "" {
		fmt.Printf("exported %d tasks to %s\n", len(tasks), *out)
	}
	return nil
}

func writeCSV(w io.Writer, tasks []Task) error {
	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "title", "priority", "due", "done", "created"})
	for _, t := range tasks {
		cw.Write([]string{
			strconv.FormatInt(t.ID, 10),
			t.Title,
			t.Priority,
			t.Due,
			strconv.FormatBool(t.Done),
			t.Created.Format(time.RFC3339),
		})
	}
	cw.Flush()
	return cw.Error()
}

func printTable(tasks []Task) {
	today := time.Now().Format("2006-01-02")
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\t \tPRI\tDUE\tTITLE")
	for _, t := range tasks {
		mark := " "
		if t.Done {
			mark = "x"
		} else if t.Overdue(today) {
			mark = "!"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", t.ID, mark, t.Priority, t.Due, t.Title)
	}
	tw.Flush()
}

func splitFirst(args []string, name string) (string, []string, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", nil, fmt.Errorf("usage: tasks <command> <%s> [flags]", name)
	}
	return args[0], args[1:], nil
}

func parseID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("id must be a positive number (got %q)", s)
	}
	return id, nil
}
