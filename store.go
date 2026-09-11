package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Task struct {
	ID       int64
	Title    string
	Priority string // low, medium, high
	Due      string // YYYY-MM-DD, or empty for no due date
	Done     bool
	Created  time.Time
}

func (t Task) Overdue(today string) bool {
	return !t.Done && t.Due != "" && t.Due < today
}

var priorities = map[string]int{"low": 1, "medium": 2, "high": 3}

func validPriority(p string) (string, error) {
	p = strings.ToLower(strings.TrimSpace(p))
	if _, ok := priorities[p]; !ok {
		return "", fmt.Errorf("priority must be low, medium or high (got %q)", p)
	}
	return p, nil
}

func validDue(d string) (string, error) {
	d = strings.TrimSpace(d)
	if d == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", d); err != nil {
		return "", fmt.Errorf("due date must look like 2006-01-02 (got %q)", d)
	}
	return d, nil
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	// busy_timeout: wait for a lock instead of failing when another tasks process is mid-write.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	// One connection keeps ":memory:" a single database in tests.
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		title    TEXT NOT NULL CHECK (title <> ''),
		priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low','medium','high')),
		due      TEXT NOT NULL DEFAULT '',
		done     INTEGER NOT NULL DEFAULT 0,
		created  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Add(title, priority, due string) (int64, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, errors.New("title cannot be empty")
	}
	priority, err := validPriority(priority)
	if err != nil {
		return 0, err
	}
	due, err = validDue(due)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO tasks (title, priority, due) VALUES (?, ?, ?)`, title, priority, due)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Get(id int64) (Task, error) {
	rows, err := s.query(`WHERE id = ?`, id)
	if err != nil {
		return Task{}, err
	}
	if len(rows) == 0 {
		return Task{}, fmt.Errorf("no task with id %d", id)
	}
	return rows[0], nil
}

type Filter struct {
	All      bool
	DoneOnly bool
	Priority string
	Sort     string // created, due, priority
}

func (s *Store) List(f Filter) ([]Task, error) {
	var where []string
	var args []any

	switch {
	case f.DoneOnly:
		where = append(where, "done = 1")
	case !f.All:
		where = append(where, "done = 0")
	}
	if f.Priority != "" {
		p, err := validPriority(f.Priority)
		if err != nil {
			return nil, err
		}
		where = append(where, "priority = ?")
		args = append(args, p)
	}

	var order string
	switch f.Sort {
	case "", "created":
		order = "created, id"
	case "due":
		// Tasks without a due date sort last.
		order = "due = '', due, id"
	case "priority":
		order = "CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END, id"
	default:
		return nil, fmt.Errorf("sort must be created, due or priority (got %q)", f.Sort)
	}

	clause := ""
	if len(where) > 0 {
		clause = "WHERE " + strings.Join(where, " AND ")
	}
	return s.query(clause+" ORDER BY "+order, args...)
}

// nil means leave that field alone.
func (s *Store) Update(id int64, title, priority, due *string) error {
	var sets []string
	var args []any

	if title != nil {
		t := strings.TrimSpace(*title)
		if t == "" {
			return errors.New("title cannot be empty")
		}
		sets = append(sets, "title = ?")
		args = append(args, t)
	}
	if priority != nil {
		p, err := validPriority(*priority)
		if err != nil {
			return err
		}
		sets = append(sets, "priority = ?")
		args = append(args, p)
	}
	if due != nil {
		d, err := validDue(*due)
		if err != nil {
			return err
		}
		sets = append(sets, "due = ?")
		args = append(args, d)
	}
	if len(sets) == 0 {
		return errors.New("nothing to update")
	}
	return s.exec(id, `UPDATE tasks SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
}

func (s *Store) SetDone(id int64, done bool) error {
	return s.exec(id, `UPDATE tasks SET done = ? WHERE id = ?`, done)
}

func (s *Store) Delete(id int64) error {
	return s.exec(id, `DELETE FROM tasks WHERE id = ?`)
}

// q must end in WHERE id = ?
func (s *Store) exec(id int64, q string, args ...any) error {
	res, err := s.db.Exec(q, append(args, id)...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no task with id %d", id)
	}
	return nil
}

func (s *Store) query(tail string, args ...any) ([]Task, error) {
	rows, err := s.db.Query(`SELECT id, title, priority, due, done, created FROM tasks `+tail, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var created string
		if err := rows.Scan(&t.ID, &t.Title, &t.Priority, &t.Due, &t.Done, &created); err != nil {
			return nil, err
		}
		t.Created, _ = time.Parse(time.RFC3339, created)
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
