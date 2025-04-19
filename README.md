# GitDB

**GitDB** is a lightweight SQL-like database engine built in Go that stores data in JSON files and tracks changes with Git. It comes with a custom JDBC driver written in Java, making it easy to integrate with standard SQL tools and test suites.

## ⚠️ Disclaimer

> **This project is experimental and not intended for production use.**

GitDB is a fun proof-of-concept it is **not tested, hardened, or optimised** for real-world usage at scale or in production environments.

**Use at your own risk.** Data loss, inconsistency, or performance issues are very possible and very probable!

---

## Project Structure

```
GitDB/
├── gitdb-server/           # Go-based backend
│   ├── main.go
│   └── ...                 
├── gitdb-jdbc-driver/      # Java JDBC driver for GitDB
│   ├── src/
│   ├── pom.xml
│   └── ...
└── Makefile
```

---

## Getting Started

### Prerequisites

- `Go`
- `Java`
- `Maven`
- `Docker`
- `Git`

---

## Build

```bash
make build
```

This will:

1. Compile the Go `gitdb-server`
2. Compile the Java JDBC driver via Maven

---

## Run Tests

```bash
make test
```

---

## Running the Server Manually

You can launch the Go server directly for debugging:

```bash
cd gitdb-server
go run main.go --root .gitdb --port 8080
```

---

## JDBC Integration Example

```java
Class.forName("com.gitdb.GitDBJDBCDriver");
Connection conn = DriverManager.getConnection("jdbc:gitdb:http://localhost:8080");

Statement stmt = conn.createStatement();
stmt.execute("CREATE DATABASE demo");
stmt.execute("USE DATABASE demo");
stmt.execute("CREATE TABLE users (name,email)");
stmt.execute("INSERT INTO users (name,email) VALUES ('Alice','alice@example.com')");
ResultSet rs = stmt.executeQuery("SELECT * FROM users");
```

---
