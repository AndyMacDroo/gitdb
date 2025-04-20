package com.gitdb;

import java.sql.*;
import java.util.*;

public class GitDBResultSetMetaData implements ResultSetMetaData {

    private final List<String> columnNames;

    public GitDBResultSetMetaData(List<Map<String, Object>> rows) {
        if (rows != null && !rows.isEmpty()) {
            columnNames = new ArrayList<>(rows.get(0).keySet());
        } else {
            columnNames = Collections.emptyList();
        }
    }

    @Override
    public int getColumnCount() {
        return columnNames.size();
    }

    @Override
    public String getColumnLabel(int column) {
        return columnNames.get(column - 1);
    }

    @Override
    public String getColumnName(int column) {
        return columnNames.get(column - 1);
    }

    @Override
    public int getColumnType(int column) {
        return Types.VARCHAR; // You could customize this if needed
    }

    @Override
    public String getColumnTypeName(int column) {
        return "VARCHAR";
    }

    @Override
    public int getColumnDisplaySize(int column) {
        return 255;
    }

    @Override public String getSchemaName(int column) { return ""; }
    @Override public String getTableName(int column) { return ""; }
    @Override public String getCatalogName(int column) { return ""; }
    @Override public int getPrecision(int column) { return 0; }
    @Override public int getScale(int column) { return 0; }
    @Override public int isNullable(int column) { return ResultSetMetaData.columnNullable; }
    @Override public boolean isAutoIncrement(int column) { return false; }
    @Override public boolean isCaseSensitive(int column) { return true; }
    @Override public boolean isSearchable(int column) { return true; }
    @Override public boolean isCurrency(int column) { return false; }
    @Override public boolean isSigned(int column) { return false; }
    @Override public boolean isReadOnly(int column) { return true; }
    @Override public boolean isWritable(int column) { return false; }
    @Override public boolean isDefinitelyWritable(int column) { return false; }
    @Override public String getColumnClassName(int column) { return "java.lang.String"; }

    @Override public <T> T unwrap(Class<T> iface) { return null; }
    @Override public boolean isWrapperFor(Class<?> iface) { return false; }
}
