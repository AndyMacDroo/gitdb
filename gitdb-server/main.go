package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"sync"
	"flag"
	"math"
)

type Row map[string]interface{}

type Session struct {
	CurrentDatabase string
	LastActive      time.Time
}

type ColType string

const (
    ColInt     ColType = "INT"
    ColFloat   ColType = "FLOAT"
    ColBool    ColType = "BOOL"
    ColString  ColType = "STRING"
    ColTime    ColType = "TIMESTAMP"
)

type Column struct {
    Name string  `json:"name"`
    Type ColType `json:"type"`
}

type GitDB struct {
	GlobalRoot string
	Schema map[string][]Column
	SessionData map[string]*Session
	mu          sync.RWMutex 
}

func NewGitDB(globalRoot string) (*GitDB, error) {
    _ = os.MkdirAll(globalRoot, 0755)
    return &GitDB{
        GlobalRoot: globalRoot,
		Schema: make(map[string][]Column),
		SessionData: make(map[string]*Session),
    }, nil
}


func (db *GitDB) dbPath(sessionID string) (string, error) {
    sess := db.GetSession(sessionID)
    if sess.CurrentDatabase == "" {
        return "", fmt.Errorf("no database selected; call USE DATABASE first")
    }
    return filepath.Join(db.GlobalRoot, sess.CurrentDatabase), nil
}

func (db *GitDB) GetSession(id string) *Session {
	db.mu.RLock()
	s, ok := db.SessionData[id]
	db.mu.RUnlock()
	if ok {
		s.LastActive = time.Now()
		return s
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if s, ok = db.SessionData[id]; ok {
		s.LastActive = time.Now()
		return s
	}
	s = &Session{LastActive: time.Now()}
	db.SessionData[id] = s
	return s
}

func (db *GitDB) CreateDatabase(name string) error {
    path := filepath.Join(db.GlobalRoot, name)
    if err := os.MkdirAll(path, 0755); err != nil {
        return err
    }
    cmd := exec.Command("git", "init")
    cmd.Dir = path
    return cmd.Run()
}

func (db *GitDB) DropDatabase(name string) error {
    path := filepath.Join(db.GlobalRoot, name)
    return os.RemoveAll(path)
}

func (db *GitDB) UseDatabase(name string, sessionID string) error {
    path := filepath.Join(db.GlobalRoot, name)
    if _, err := os.Stat(path); os.IsNotExist(err) {
        return fmt.Errorf("database %s does not exist", name)
    }

	db.GetSession(sessionID).CurrentDatabase = name

    if _, err := os.Stat(filepath.Join(path, ".git")); os.IsNotExist(err) {
        cmd := exec.Command("git", "init")
        cmd.Dir = path
        if err := cmd.Run(); err != nil {
            return fmt.Errorf("failed to init git: %w", err)
        }
    }

    return nil
}

func (db *GitDB) CreateTable(name string, columns []Column, sessionID string) error {
    root, err := db.dbPath(sessionID)
    if err != nil {
        return err
    }

    dir := filepath.Join(root, name)
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }

    seen := map[string]struct{}{}
    for i, c := range columns {
        if c.Name == "" {
            return fmt.Errorf("column %d has no name", i)
        }
        c.Type = ColType(strings.ToUpper(string(c.Type)))
        switch c.Type {
        case ColInt, ColFloat, ColBool, ColString, ColTime:
        default:
            return fmt.Errorf("unsupported type %q", c.Type)
        }
        if _, dup := seen[c.Name]; dup {
            return fmt.Errorf("duplicate column %q", c.Name)
        }
        seen[c.Name] = struct{}{}
        columns[i] = c
    }

    schemaFile := filepath.Join(dir, "_schema.json")
    if err := func() error {
        f, err := os.Create(schemaFile)
        if err != nil {
            return err
        }
        defer f.Close()
        return json.NewEncoder(f).Encode(columns)
    }(); err != nil {
        return err
    }

    db.mu.Lock()
    db.Schema[name] = columns
    db.mu.Unlock()

    return db.gitCommit(root, "Create table "+name)
}

func (db *GitDB) DropTable(name string, sessionID string) error {
    root, err := db.dbPath(sessionID)
    if err != nil {
        return err 
    }
    dir := filepath.Join(root, name)
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }
	_ = os.RemoveAll(dir)
	return db.gitCommit(root, "Drop table "+name)
}

func (db *GitDB) TruncateTable(name string, sessionID string) error {
    root, err := db.dbPath(sessionID)
    if err != nil {
        return err 
    }
    dir := filepath.Join(root, name)
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }
	files, _ := ioutil.ReadDir(dir)
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") && !strings.HasPrefix(f.Name(), "_") {
			_ = os.Remove(filepath.Join(dir, f.Name()))
		}
	}
	return db.gitCommit(root, "Truncate table "+name)
}

func zeroValue(ct ColType) interface{} {
    switch ct {
    case ColInt, ColFloat:
        return 0
    case ColBool:
        return false
    case ColTime:
        return time.Time{}
    default:
        return nil
    }
}

func (db *GitDB) Insert(table string, row Row, sessionID string) error {
    db.mu.RLock()
    cols, ok := db.Schema[table]
    db.mu.RUnlock()
    if !ok {
        return fmt.Errorf("table %q does not exist", table)
    }
    colMap := map[string]ColType{}
    for _, c := range cols {
        colMap[c.Name] = c.Type
    }

    for fld, raw := range row {
        ct, exists := colMap[fld]
        if !exists {
            return fmt.Errorf("unknown column %q", fld)
        }
        v, err := convert(fmt.Sprint(raw), ct)
        if err != nil {
            return fmt.Errorf("%s: %w", fld, err)
        }
        row[fld] = v
    }

    for _, c := range cols {
        if _, ok := row[c.Name]; !ok {
            row[c.Name] = zeroValue(c.Type)
        }
    }

    id := fmt.Sprintf("%d", time.Now().UnixNano())
    row["id"]         = id
    row["deleted"]    = false
    row["created_at"] = time.Now().Format(time.RFC3339)

    return db.writeRow(table, id, row, "Insert row", sessionID)
}

func (db *GitDB) Update(table, id string, updates Row, sessionID string) error {
    row, err := db.readRow(table, id, sessionID)
    if err != nil {
        return err
    }

    db.mu.RLock()
    cols, ok := db.Schema[table]
    db.mu.RUnlock()
    if !ok {
        return fmt.Errorf("table %q does not exist", table)
    }
    colMap := map[string]ColType{}
    for _, c := range cols {
        colMap[c.Name] = c.Type
    }

    for fld, raw := range updates {
        ct, exists := colMap[fld]
        if !exists {
            return fmt.Errorf("unknown column %q (run ALTER TABLE ADD COLUMN first)", fld)
        }
        v, err := convert(fmt.Sprint(raw), ct)
        if err != nil {
            return fmt.Errorf("%s: %w", fld, err)
        }
        row[fld] = v
    }
    row["updated_at"] = time.Now().Format(time.RFC3339)

    return db.writeRow(table, id, row, "Update row", sessionID)
}


func (db *GitDB) Delete(table, id string, sessionID string) error {
	row, err := db.readRow(table, id, sessionID)
	if err != nil {
		return err
	}
	row["deleted"] = true
	row["deleted_at"] = time.Now().Format(time.RFC3339)
	return db.writeRow(table, id, row, "Soft delete row", sessionID)
}

func canonicalKey(v interface{}) string {
    switch t := v.(type) {
    case float64:
        if t == math.Trunc(t) {
            return strconv.FormatInt(int64(t), 10)
        }
        return strconv.FormatFloat(t, 'f', -1, 64)
    default:
        return fmt.Sprint(t)
    }
}

func (db *GitDB) Query(table, where, orderBy, orderDir string, limit, offset int, sessionID string) ([]Row, error) {
    root, err := db.dbPath(sessionID)
    if err != nil {
        return []Row{}, err 
    }
    dir := filepath.Join(root, table)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
        return nil, fmt.Errorf("table %q does not exist", table)
    }
	files, _ := ioutil.ReadDir(dir)
	var results []Row
	conds := parseWhereClause(where)

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") || strings.HasPrefix(f.Name(), "_") {
			continue
		}

		data, err := ioutil.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}

		var row Row
		if err := json.Unmarshal(data, &row); err != nil {
			continue
		}
	
		if deleted, ok := row["deleted"].(bool); ok && deleted {
			continue
		}

		if matchesConditions(row, conds) {
			results = append(results, row)
		}
	}

	if orderBy != "" {
		desc := strings.ToUpper(orderDir) == "DESC"
		sort.Slice(results, func(i, j int) bool {
			less := valueLess(results[i][orderBy], results[j][orderBy])
			if desc {
				return !less
			}
			return less
		})
	}

	if offset > len(results) {
		return []Row{}, nil
	}
	results = results[offset:]
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return results, nil
}

func (db *GitDB) AlterTableAddColumn(table string, col Column, sessionID string) error {
    root, err := db.dbPath(sessionID)
    if err != nil {
        return err
    }
    tableDir   := filepath.Join(root, table)
    schemaPath := filepath.Join(tableDir, "_schema.json")

    var columns []Column
    data, err := ioutil.ReadFile(schemaPath)
    if err != nil {
        return err
    }
    if err := json.Unmarshal(data, &columns); err != nil {
        return err
    }

    for _, c := range columns {
        if c.Name == col.Name {
            return fmt.Errorf("column %q already exists in table %q", col.Name, table)
        }
    }
    col.Type = ColType(strings.ToUpper(string(col.Type)))
    switch col.Type {
    case ColInt, ColFloat, ColBool, ColString, ColTime:
    default:
        return fmt.Errorf("unsupported type %q", col.Type)
    }

    columns = append(columns, col)
    newSchema, _ := json.MarshalIndent(columns, "", "  ")
    if err := ioutil.WriteFile(schemaPath, newSchema, 0o644); err != nil {
        return err
    }

    zero := func(ct ColType) interface{} {
        switch ct {
        case ColInt, ColFloat:
            return 0
        case ColBool:
            return false
        case ColTime:
            return time.Time{}
        default:
            return nil
        }
    }(col.Type)

    files, _ := ioutil.ReadDir(tableDir)
    for _, f := range files {
        if !strings.HasSuffix(f.Name(), ".json") || strings.HasPrefix(f.Name(), "_") {
            continue
        }
        rowPath := filepath.Join(tableDir, f.Name())
        b, err := ioutil.ReadFile(rowPath)
        if err != nil {
            continue
        }

        var row Row
        if err := json.Unmarshal(b, &row); err != nil {
            continue
        }

        if _, exists := row[col.Name]; !exists {
            row[col.Name] = zero
            newData, _ := json.MarshalIndent(row, "", "  ")
            _ = ioutil.WriteFile(rowPath, newData, 0o644)
        }
    }

    db.mu.Lock()
    db.Schema[table] = columns
    db.mu.Unlock()

    return db.gitCommit(root,
        fmt.Sprintf("Alter table %s: add column %s %s", table, col.Name, col.Type))
}


func valueLess(a, b interface{}) bool {
    if a == nil && b != nil { return true }
    if b == nil { return false }

    switch av := a.(type) {
    case int:
        switch bv := b.(type) {
        case int:       return av < bv
        case float64:   return float64(av) < bv
        }
    case int64:
        switch bv := b.(type) {
        case int64:     return av < bv
        case float64:   return float64(av) < bv
        }
    case float64:
        switch bv := b.(type) {
        case int:       return av < float64(bv)
        case int64:     return av < float64(bv)
        case float64:   return av < bv
        }
    case bool:
        if bv, ok := b.(bool); ok {
            return !av && bv
        }
    case time.Time:
        if bv, ok := b.(time.Time); ok {
            return av.Before(bv)
        }
    case string:
        if bv, ok := b.(string); ok {
            return av < bv
        }
    }

    return fmt.Sprint(a) < fmt.Sprint(b)
}

func convert(val string, typ ColType) (interface{}, error) {
    switch typ {
    case ColInt:
        return strconv.Atoi(val)
    case ColFloat:
        return strconv.ParseFloat(val, 64)
    case ColBool:
        return strconv.ParseBool(val)
    case ColTime:
        return time.Parse(time.RFC3339, val)
    default:
        return val, nil
    }
}

func parseWhereClause(where string) map[string]string {
	conds := map[string]string{}
	if where == "" {
		return conds
	}

	parts := strings.Split(where, " AND ")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		if strings.Contains(part, "LIKE") {
			kv := strings.SplitN(part, "LIKE", 2)
			if len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				value := strings.Trim(kv[1], " '\")")
				conds[key] = value
			}
		} else {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				value := strings.Trim(kv[1], " '\")")
				conds[key] = value
			}
		}
	}

	return conds
}


func matchesConditions(row Row, conds map[string]string) bool {
	for k, v := range conds {
		fieldValue := fmt.Sprint(row[k])

		if strings.Contains(v, "%") {

			regexPattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(v), "%", ".*") + "$"
			matched, err := regexp.MatchString(regexPattern, fieldValue)
			if err != nil || !matched {
				return false
			}
		} else {
			if fieldValue != v {
				return false
			}
		}
	}
	return true
}


func (db *GitDB) Join(left, right, lkey, rkey string, sessionID string) ([]map[string]interface{}, error) {
    lrows, err := db.Query(left,  "", "", "", 0, 0, sessionID)
    if err != nil { return nil, err }

    rrows, err := db.Query(right, "", "", "", 0, 0, sessionID)
    if err != nil { return nil, err }

    rindex := map[string][]Row{}
    for _, r := range rrows {
        key := canonicalKey(r[rkey])
        rindex[key] = append(rindex[key], r)
    }

    var result []map[string]interface{}
    for _, l := range lrows {
        matches := rindex[canonicalKey(l[lkey])]
        for _, r := range matches {
            merged := map[string]interface{}{}
            for k, v := range l { merged[left+"."+k]  = v }
            for k, v := range r { merged[right+"."+k] = v }
            result = append(result, merged)
        }
    }

    if result == nil { result = []map[string]interface{}{} }
    return result, nil
}

func (db *GitDB) writeRow(table, id string, row Row, msg string, sessionID string) error {
    root, err := db.dbPath(sessionID)
    if err != nil {
        return err 
    }
    dir := filepath.Join(root, table)
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }

	b, _ := json.MarshalIndent(row, "", "  ")
	path := filepath.Join(dir, id+".json")
	_ = ioutil.WriteFile(path, b, 0644)

	preview := strings.Builder{}
	for k, v := range row {
		preview.WriteString(fmt.Sprintf("- %s: %v\n", k, v))
	}

	commitMsg := fmt.Sprintf(
		"%s\n\nTable: %s\nRow ID: %s\n\nData:\n%s",
		msg, table, id, preview.String(),
	)

	return db.gitCommit(dir, commitMsg)
}

func (db *GitDB) readRow(table, id string, sessionID string) (Row, error) {
    root, err := db.dbPath(sessionID)
	var row Row
    if err != nil {
        return row, err 
    }
	path := filepath.Join(root, table, id+".json")
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(b, &row)
	return row, nil
}

func (db *GitDB) gitCommit(root, msg string) error {
	cmdAdd := exec.Command("git", "add", ".")
	cmdAdd.Dir = root
	if err := cmdAdd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	cmdCommit := exec.Command("git", "commit", "-m", msg)
	cmdCommit.Dir = root
	var out, errBuf bytes.Buffer
	cmdCommit.Stdout = &out
	cmdCommit.Stderr = &errBuf
	if err := cmdCommit.Run(); err != nil {
		return fmt.Errorf("git commit failed: %s\nOutput: %s", errBuf.String(), out.String())
	}
	return nil
}

func (db *GitDB) executeStatement(sql string, sessionID string) (any, error) {
	sql = strings.TrimSpace(sql)

	createDB := regexp.MustCompile(`(?i)^CREATE DATABASE (\w+)$`)
	dropDB := regexp.MustCompile(`(?i)^DROP DATABASE (\w+)$`)
	useDB := regexp.MustCompile(`(?i)^USE DATABASE (\w+)$`)
	createTable := regexp.MustCompile(`(?i)^CREATE TABLE (\w+) \((.+)\)$`)
	dropTable := regexp.MustCompile(`(?i)^DROP TABLE (\w+)$`)
	truncate := regexp.MustCompile(`(?i)^TRUNCATE TABLE (\w+)$`)
	insert := regexp.MustCompile(`(?i)^INSERT INTO (\w+) \((.+)\) VALUES \((.+)\)$`)
	deleteWhere := regexp.MustCompile(`(?i)^DELETE FROM (\w+)\s+WHERE\s+(.+)$`)
	selectRe := regexp.MustCompile(`(?i)^SELECT\s+(DISTINCT\s+)?(.+?)\s+FROM\s+(\w+)(?:\s+WHERE\s+(.+?))?(?:\s+ORDER BY\s+(\w+)(?:\s+(ASC|DESC))?)?(?:\s+LIMIT\s+(\d+))?(?:\s+OFFSET\s+(\d+))?$`)
	joinRe := regexp.MustCompile(`(?i)^SELECT \* FROM (\w+) JOIN (\w+) ON (\w+)\.(\w+)=\w+\.(\w+)$`)
	alterAddColumn := regexp.MustCompile(`(?i)^ALTER TABLE (\w+) ADD COLUMN (\w+)\s+(\w+)$`)

	switch {
	case createDB.MatchString(sql):
		m := createDB.FindStringSubmatch(sql)
		return map[string]string{"status": "ok"}, db.CreateDatabase(m[1])
	case dropDB.MatchString(sql):
		m := dropDB.FindStringSubmatch(sql)
		return map[string]string{"status": "ok"}, db.DropDatabase(m[1])
	case useDB.MatchString(sql):
		m := useDB.FindStringSubmatch(sql)
		return map[string]string{"status": "ok"}, db.UseDatabase(m[1], sessionID)
	case createTable.MatchString(sql):
		m := createTable.FindStringSubmatch(sql)
		rawCols := strings.Split(m[2], ",")
		cols := make([]Column, len(rawCols))
		for i, spec := range rawCols {
			parts := strings.Fields(strings.TrimSpace(spec))
			if len(parts) != 2 {
				return nil, fmt.Errorf("bad column spec %q", spec)
			}
			cols[i] = Column{Name: parts[0], Type: ColType(strings.ToUpper(parts[1]))}
		}
		return map[string]string{"status": "ok"}, db.CreateTable(m[1], cols, sessionID)
	case dropTable.MatchString(sql):
		m := dropTable.FindStringSubmatch(sql)
		return map[string]string{"status": "ok"}, db.DropTable(m[1], sessionID)
	case truncate.MatchString(sql):
		m := truncate.FindStringSubmatch(sql)
		return map[string]string{"status": "ok"}, db.TruncateTable(m[1], sessionID)
	case insert.MatchString(sql):
		m := insert.FindStringSubmatch(sql)
		fields := strings.Split(m[2], ",")
		values := strings.Split(m[3], ",")
		row := Row{}
		for i := range fields {
			row[strings.TrimSpace(fields[i])] = strings.Trim(values[i], " '\")")
		}
		return map[string]string{"status": "ok"}, db.Insert(m[1], row, sessionID)
	case strings.HasPrefix(strings.ToUpper(sql), "UPDATE "):
		updateFull := regexp.MustCompile(`(?i)^UPDATE (\w+)\s+SET\s+(.+?)\s+WHERE\s+(.+)$`)
		if m := updateFull.FindStringSubmatch(sql); m != nil {
			table := m[1]
			setClause := m[2]
			whereClause := m[3]

			updates := Row{}
			for _, p := range strings.Split(setClause, ",") {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) == 2 {
					key := strings.TrimSpace(kv[0])
					val := strings.Trim(kv[1], " '\")")
					updates[key] = val
				}
			}

			rows, err := db.Query(table, whereClause, "", "", 0, 0, sessionID)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				id, ok := row["id"].(string)
				if !ok || id == "" {
					continue
				}
				if err := db.Update(table, id, updates, sessionID); err != nil {
					return nil, err
				}
			}
			return map[string]string{"status": "ok"}, nil
		}
	case deleteWhere.MatchString(sql):
		m := deleteWhere.FindStringSubmatch(sql)
		table := m[1]
		whereClause := m[2]
		rows, err := db.Query(table, whereClause, "", "", 0, 0, sessionID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			id, ok := row["id"].(string)
			if !ok || id == "" {
				continue
			}
			if err := db.Delete(table, id, sessionID); err != nil {
				return nil, err
			}
		}
		return map[string]string{"status": "ok"}, nil
	case joinRe.MatchString(sql):
		m := joinRe.FindStringSubmatch(sql)
		return db.Join(m[1], m[2], m[4], m[5], sessionID)
	case alterAddColumn.MatchString(sql): 
		m := alterAddColumn.FindStringSubmatch(sql) 
		col := Column{Name: m[2], Type: ColType(strings.ToUpper(m[3]))} 
		return map[string]string{"status": "ok"}, db.AlterTableAddColumn(m[1], col, sessionID)
	case selectRe.MatchString(sql):
		m := selectRe.FindStringSubmatch(sql)
		distinct := m[1] != ""
		columns := strings.Split(m[2], ",")
		for i := range columns {
			columns[i] = strings.TrimSpace(columns[i])
		}
		table := m[3]
		where := m[4]
		orderBy := m[5]
		orderDir := m[6]
		limit, _ := strconv.Atoi(m[7])
		offset, _ := strconv.Atoi(m[8])
	
		rows, err := db.Query(table, where, orderBy, orderDir, limit, offset, sessionID)
		if err != nil {
			return nil, err
		}
	
		var result []map[string]interface{}
		seen := map[string]bool{}
	
		for _, r := range rows {
			filtered := map[string]interface{}{}
			if columns[0] == "*" {
				filtered = make(map[string]interface{})
				for k, v := range r {
					if k != "deleted" && k != "deleted_at" {
						filtered[k] = v
					}
				}
			} else {
				for _, c := range columns {
					filtered[c] = r[c]
				}
			}
	
			if distinct {
				key := fmt.Sprint(filtered)
				if seen[key] {
					continue
				}
				seen[key] = true
			}
	
			result = append(result, filtered)
		}
		if result == nil {
			result = []map[string]interface{}{}
		}
		return result, nil	
	}
	return nil, fmt.Errorf("unsupported SQL: %s", sql)
}

func (db *GitDB) ExecuteSQL(statement, sessionID string) (any, error) {
    statements := strings.Split(statement, ";")
    var results []any

    useDB := regexp.MustCompile(`(?i)^USE DATABASE ([a-zA-Z0-9_-]+)$`)

    for _, stmt := range statements {
        stmt = strings.TrimSpace(stmt)
        if stmt == "" {
            continue
        }

        if useDB.MatchString(stmt) {
            m := useDB.FindStringSubmatch(stmt)
            if err := db.UseDatabase(m[1], sessionID); err != nil {
                return nil, err
            }
            results = append(results, map[string]string{"status": "ok"})
            continue
        }

        result, err := db.executeStatement(stmt, sessionID)
        if err != nil {
            return nil, err
        }
        results = append(results, result)
    }
    return results, nil
}

func (db *GitDB) loadSchemas() error {
	return filepath.Walk(db.GlobalRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == "_schema.json" {
			rel, err := filepath.Rel(db.GlobalRoot, filepath.Dir(path))
			if err != nil {
				return err
			}
			data, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			var columns []Column
			if err := json.Unmarshal(data, &columns); err != nil {
				return err
			}
			db.mu.Lock()
			db.Schema[rel] = columns
			db.mu.Unlock()
		}
		return nil
	})
}

func main() {
	root := flag.String("root", ".gitdb", "Path to the GitDB root directory")
	port := flag.String("port", "8080", "Port to run GitDB server on")
	flag.Parse()

	db, _ := NewGitDB(*root)
	if err := db.loadSchemas(); err != nil {
		fmt.Println("❌ Failed to load schema:", err)
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			db.mu.Lock()
			for id, sess := range db.SessionData {
				if time.Since(sess.LastActive) > 30*time.Minute {
					delete(db.SessionData, id)
					fmt.Println("🧹 Purged inactive session:", id)
				}
			}
			db.mu.Unlock()
		}
	}()
	
	http.HandleFunc("/sql", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ SQL string `json:"sql"` }
		json.NewDecoder(r.Body).Decode(&q)
		sessionID := r.Header.Get("Session-ID")
		fmt.Printf("[Session: %s]: %s\n", sessionID, q.SQL)
		result, err := db.ExecuteSQL(q.SQL, sessionID)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			fmt.Println("❌ ERROR:", err.Error()) 
			return 
		}
		json.NewEncoder(w).Encode(result)
	})
	addr := ":" + *port
	fmt.Printf("🚀 GitDB listening on %s (root: %s)\n", addr, *root)
	_ = http.ListenAndServe(addr, nil)
}
