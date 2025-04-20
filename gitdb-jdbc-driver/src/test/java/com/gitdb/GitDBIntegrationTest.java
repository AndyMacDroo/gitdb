package com.gitdb;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.time.Duration;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.UUID;

import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestInfo;
import org.junit.jupiter.api.TestInstance;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.wait.strategy.Wait;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@TestInstance(TestInstance.Lifecycle.PER_CLASS)
@Testcontainers
public class GitDBIntegrationTest {

    private Connection conn;
    private String dbName;

    @Container
    static GenericContainer<?> goServer = new GenericContainer<>("golang:1.21")
        .withExposedPorts(8080)
        .withFileSystemBind("../gitdb-server", "/app")
        .withFileSystemBind("/tmp/gitdb-data", "/data")
        .withWorkingDirectory("/app")
        .withCommand("/bin/sh", "-c",
        "git config --global user.email 'test@example.com' && " +
        "git config --global user.name 'GitDB Test' && " +
        "go mod tidy && go build -o server . && ./server --root /data --port 8080")
        .waitingFor(Wait.forHttp("/sql").forStatusCode(200).withStartupTimeout(Duration.ofSeconds(20)));
    

    @BeforeAll
    void setUp() throws Exception {
        String mappedPort = String.valueOf(goServer.getMappedPort(8080));
        Class.forName("com.gitdb.GitDBJDBCDriver");
        conn = DriverManager.getConnection("jdbc:gitdb:http://localhost:" + mappedPort);

        dbName = "testdb_" + UUID.randomUUID().toString().replace("-", "");

        Statement stmt = conn.createStatement();
        stmt.execute("CREATE DATABASE " + dbName);
        stmt.execute("USE DATABASE " + dbName);
        stmt.execute("CREATE TABLE users (name STRING, email STRING)");
        stmt.execute("CREATE TABLE orders (user_id STRING, product STRING, total INT)");

        stmt.execute("INSERT INTO users (name,email) VALUES (\"Alice\",\"alice@example.com\")");
        stmt.execute("INSERT INTO users (name,email) VALUES (\"Bob\",\"bob@example.com\")");

        ResultSet rs = stmt.executeQuery("SELECT * FROM users WHERE name=\"Alice\"");
        if (rs.next()) {
            String userId = rs.getString("id");
            stmt.execute("INSERT INTO orders (user_id,product,total) VALUES (" + userId + ",\"Widget\",100)");
            stmt.execute("UPDATE users SET name=\"Alice Updated\" WHERE id=\"" + userId + "\"");
        }

        for (int i = 1; i <= 100; i++) {
            stmt.execute(String.format("INSERT INTO users (name,email) VALUES ('User%d','user%d@example.com')", i, i));
        }
    }

    @AfterEach
    void logContainerOutput(TestInfo info) {
        if (goServer != null && goServer.isRunning()) {
            System.out.println("========= Container logs for test: " + info.getDisplayName() + " =========");
            System.out.println(goServer.getLogs());
        }
    }

    @AfterAll
    void tearDown() throws Exception {
        if (conn != null) {
            conn.close();
        }
    }

    @Test
    void queryUsers() throws Exception {
        Statement stmt = conn.createStatement();
        ResultSet rs = stmt.executeQuery("SELECT * FROM users");
        List<String> names = new ArrayList<>();
        while (rs.next()) {
            names.add(rs.getString("name"));
        }
        Assertions.assertTrue(names.contains("Alice Updated"));
        Assertions.assertTrue(names.contains("Bob"));
        Assertions.assertTrue(names.contains("User50"));
        Assertions.assertEquals(102, names.size());
    }

    @Test
    void queryWithLimitOffsetOrder() throws Exception {
        Statement stmt = conn.createStatement();
        ResultSet rs = stmt.executeQuery("SELECT * FROM users ORDER BY name ASC LIMIT 5 OFFSET 95");
        List<String> subset = new ArrayList<>();
        while (rs.next()) {
            subset.add(rs.getString("name"));
        }
        Assertions.assertEquals(5, subset.size());
    }

    @Test
    void verifyJoinWithOrders() throws Exception {
        Statement stmt = conn.createStatement();
        ResultSet rs = stmt.executeQuery("SELECT * FROM users JOIN orders ON users.id=orders.user_id");

        boolean found = false;
        while (rs.next()) {
            if ("Alice Updated".equals(rs.getString("users.name")) && "Widget".equals(rs.getString("orders.product"))) {
                found = true;
                break;
            }
        }
        Assertions.assertTrue(found, "Expected join result not found");
    }

    @Test
    void testDistinct() throws Exception {
        Statement stmt = conn.createStatement();
        ResultSet rs = stmt.executeQuery("SELECT DISTINCT name FROM users WHERE name LIKE 'User%'");
        Set<String> names = new HashSet<>();
        while (rs.next()) {
            names.add(rs.getString("name"));
        }
        Assertions.assertEquals(100, names.size());
    }

    @Test
    void deleteUserAndVerify() throws Exception {
        Statement stmt = conn.createStatement();

        String uniqueName = "TempDeleteUser";
        stmt.execute("INSERT INTO users (name,email) VALUES (\"" + uniqueName + "\",\"temp@example.com\")");

        ResultSet rs = stmt.executeQuery("SELECT * FROM users WHERE name=\"" + uniqueName + "\"");
        String tempId = null;
        if (rs.next()) {
            tempId = rs.getString("id");
        }
        Assertions.assertNotNull(tempId);

        stmt.execute("DELETE FROM users WHERE id=\"" + tempId + "\"");

        ResultSet rs2 = stmt.executeQuery("SELECT * FROM users WHERE id=\"" + tempId + "\"");
        Assertions.assertFalse(rs2.next(), "User should have been deleted");
    }

    @Test
    void multipleStatementsExecution() throws Exception {
        Statement stmt = conn.createStatement();
        stmt.execute("CREATE TABLE multi1 (a STRING); CREATE TABLE multi2 (b STRING);");

        ResultSet rs1 = stmt.executeQuery("SELECT * FROM multi1");
        Assertions.assertFalse(rs1.next());

        ResultSet rs2 = stmt.executeQuery("SELECT * FROM multi2");
        Assertions.assertFalse(rs2.next());

        stmt.execute("DROP TABLE multi1");
        stmt.execute("DROP TABLE multi2");
    }

    @Test
    void createAndDropTable() throws Exception {
        Statement stmt = conn.createStatement();
        stmt.execute("CREATE TABLE temp (x INT,y INT)");
        stmt.execute("INSERT INTO temp (x,y) VALUES (\"1\",\"2\")");

        ResultSet rs = stmt.executeQuery("SELECT * FROM temp");
        Assertions.assertTrue(rs.next());

        stmt.execute("DROP TABLE temp");

        SQLException ex = Assertions.assertThrows(SQLException.class, () ->
                stmt.executeQuery("SELECT * FROM temp")
        );
        Assertions.assertTrue(ex.getMessage().contains("Error executing SQL"));
    }

    @Test
    void updateNewColumnValue() throws Exception {
        Statement stmt = conn.createStatement();

        stmt.execute("ALTER TABLE users ADD COLUMN nickname STRING");
        stmt.execute("UPDATE users SET nickname=\"bobby\" WHERE name=\"Bob\"");

        ResultSet rs = stmt.executeQuery("SELECT nickname FROM users WHERE name=\"Bob\"");
        Assertions.assertTrue(rs.next());
        Assertions.assertEquals("bobby", rs.getString("nickname"));
    }

    @Test
    void alterTableAddColumnAndVerify() throws Exception {
        Statement stmt = conn.createStatement();
        stmt.execute("ALTER TABLE users ADD COLUMN age INT");

        ResultSet rs = stmt.executeQuery("SELECT * FROM users WHERE name=\"Bob\"");
        Assertions.assertTrue(rs.next());
        Assertions.assertDoesNotThrow(() -> rs.getObject("age"));
    }

    @Test
    void whereInSubquery() throws Exception {
        Statement stmt = conn.createStatement();

        stmt.execute("CREATE TABLE nicknames (nickname STRING)");
        stmt.execute("CREATE TABLE people (name STRING, email STRING, nickname STRING)");
        stmt.execute("INSERT INTO people (name,email,nickname) VALUES (\"Charlie\",\"charlie@example.com\",\"char\")");
        stmt.execute("INSERT INTO people (name,email,nickname) VALUES (\"Dana\",\"dana@example.com\",\"d\")");
        stmt.execute("INSERT INTO people (name,email,nickname) VALUES (\"Eve\",\"eve@example.com\",\"eve\")");

        stmt.execute("INSERT INTO nicknames (nickname) VALUES (\"char\")");
        stmt.execute("INSERT INTO nicknames (nickname) VALUES (\"eve\")");

        ResultSet rs = stmt.executeQuery("SELECT name FROM people WHERE nickname IN (SELECT nickname FROM nicknames)");

        Set<String> results = new HashSet<>();
        while (rs.next()) {
            results.add(rs.getString("name"));
        }
        Assertions.assertTrue(results.contains("Charlie"));
        Assertions.assertTrue(results.contains("Eve"));
        Assertions.assertFalse(results.contains("Dana"));
        Assertions.assertEquals(2, results.size());
    }
}
