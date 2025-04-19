package com.gitdb;

import java.sql.*;
import java.util.Properties;
import java.util.logging.Logger;

public class GitDBJDBCDriver implements Driver {

    static {
        try {
            DriverManager.registerDriver(new GitDBJDBCDriver());
        } catch (SQLException e) {
            throw new RuntimeException("Failed to register GitDB driver", e);
        }
    }

    @Override
    public Connection connect(String url, Properties info) throws SQLException {
        if (!acceptsURL(url)) return null;
        String endpoint = url.replace("jdbc:gitdb:", "");
        return new GitDBConnection(endpoint);
    }

    @Override
    public boolean acceptsURL(String url) {
        return url != null && url.startsWith("jdbc:gitdb:");
    }

    @Override public DriverPropertyInfo[] getPropertyInfo(String url, Properties info) { return new DriverPropertyInfo[0]; }
    @Override public int getMajorVersion() { return 1; }
    @Override public int getMinorVersion() { return 0; }
    @Override public boolean jdbcCompliant() { return false; }
    @Override public Logger getParentLogger() { return Logger.getGlobal(); }
}
