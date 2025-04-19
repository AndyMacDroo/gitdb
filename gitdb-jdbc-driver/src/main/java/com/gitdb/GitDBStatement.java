package com.gitdb;

import java.sql.*;
import java.util.*;
import java.net.HttpURLConnection;
import java.net.URL;
import java.io.*;

public class GitDBStatement implements Statement {
    private final GitDBConnection connection;
    private GitDBResultSet currentResultSet;
    private int updateCount = -1;

    public GitDBStatement(GitDBConnection connection) {
        this.connection = connection;
    }

    @Override
    public boolean execute(String sql) throws SQLException {
        try {
            URL url = new URL(connection.getEndpoint() + "/sql");
            HttpURLConnection conn = (HttpURLConnection) url.openConnection();
            conn.setRequestMethod("POST");
            conn.setRequestProperty("Content-Type", "application/json");
            conn.setRequestProperty("Session-ID", connection.getSessionId());
            conn.setDoOutput(true);

            String json = "{\"sql\": \"" + sql.replace("\"", "\\\"") + "\"}";
            try (OutputStream os = conn.getOutputStream()) {
                os.write(json.getBytes());
                os.flush();
            }

            BufferedReader in = new BufferedReader(new InputStreamReader(conn.getInputStream()));
            StringBuilder response = new StringBuilder();
            String line;
            while ((line = in.readLine()) != null) {
                response.append(line);
            }
            in.close();

            String resp = response.toString().trim();
            if (resp.startsWith("[")) {
                List<Map<String, Object>> rows = GitDBResultSet.parseJson(resp);
                currentResultSet = new GitDBResultSet(rows);
                updateCount = -1;
                return true;
            } else if (resp.startsWith("{")) {
                Map<String, Object> result = GitDBResultSet.parseJsonObject(resp);
                updateCount = "ok".equals(result.get("status")) ? 1 : 0;
                currentResultSet = null;
                return false;
            } else {
                throw new SQLException("Unexpected response from server: " + resp);
            }
        } catch (Exception e) {
            throw new SQLException("Error executing SQL", e);
        }
    }

    @Override public ResultSet executeQuery(String sql) throws SQLException {
        if (!execute(sql)) throw new SQLException("Query did not return a ResultSet");
        return currentResultSet;
    }

    @Override public int executeUpdate(String sql) throws SQLException { execute(sql); return updateCount; }
    @Override public int getUpdateCount() { return updateCount; }
    @Override public ResultSet getResultSet() { return currentResultSet; }
    @Override public void close() {}
    @Override public Connection getConnection() { return connection; }

    @Override public boolean getMoreResults() { return false; }
    @Override public boolean isClosed() { return false; }
    @Override public void setFetchDirection(int direction) {}
    @Override public int getFetchDirection() { return ResultSet.FETCH_FORWARD; }
    @Override public int getFetchSize() { return 0; }
    @Override public void setFetchSize(int rows) {}
    @Override public int getResultSetConcurrency() { return ResultSet.CONCUR_READ_ONLY; }
    @Override public int getResultSetType() { return ResultSet.TYPE_FORWARD_ONLY; }
    @Override public void cancel() {}
    @Override public SQLWarning getWarnings() { return null; }
    @Override public void clearWarnings() {}
    @Override public void setCursorName(String name) {}
    @Override public void setPoolable(boolean poolable) {}
    @Override public boolean isPoolable() { return false; }
    @Override public void closeOnCompletion() {}
    @Override public boolean isCloseOnCompletion() { return false; }
    @Override public <T> T unwrap(Class<T> iface) { return null; }
    @Override public boolean isWrapperFor(Class<?> iface) { return false; }

    @Override
    public int getMaxFieldSize() throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'getMaxFieldSize'");
    }

    @Override
    public void setMaxFieldSize(int max) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'setMaxFieldSize'");
    }

    @Override
    public int getMaxRows() throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'getMaxRows'");
    }

    @Override
    public void setMaxRows(int max) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'setMaxRows'");
    }

    @Override
    public void setEscapeProcessing(boolean enable) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'setEscapeProcessing'");
    }

    @Override
    public int getQueryTimeout() throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'getQueryTimeout'");
    }

    @Override
    public void setQueryTimeout(int seconds) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'setQueryTimeout'");
    }

    @Override
    public void addBatch(String sql) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'addBatch'");
    }

    @Override
    public void clearBatch() throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'clearBatch'");
    }

    @Override
    public int[] executeBatch() throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'executeBatch'");
    }

    @Override
    public boolean getMoreResults(int current) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'getMoreResults'");
    }

    @Override
    public ResultSet getGeneratedKeys() throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'getGeneratedKeys'");
    }

    @Override
    public int executeUpdate(String sql, int autoGeneratedKeys) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'executeUpdate'");
    }

    @Override
    public int executeUpdate(String sql, int[] columnIndexes) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'executeUpdate'");
    }

    @Override
    public int executeUpdate(String sql, String[] columnNames) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'executeUpdate'");
    }

    @Override
    public boolean execute(String sql, int autoGeneratedKeys) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'execute'");
    }

    @Override
    public boolean execute(String sql, int[] columnIndexes) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'execute'");
    }

    @Override
    public boolean execute(String sql, String[] columnNames) throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'execute'");
    }

    @Override
    public int getResultSetHoldability() throws SQLException {

        throw new UnsupportedOperationException("Unimplemented method 'getResultSetHoldability'");
    }
}
