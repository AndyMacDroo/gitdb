
JDBC_DIR=gitdb-jdbc-driver
SERVER_DIR=gitdb-server

.DEFAULT_GOAL := build

.PHONY: build test clean

build:
	@echo "🔧 Building GitDB JDBC driver..."
	cd $(JDBC_DIR) && mvn clean package -DskipTests

	@echo "🔧 Building GitDB server..."
	cd $(SERVER_DIR) && go build -o gitdb-server

test: build
	@echo "🧪 Running tests for GitDB server..."
	cd $(SERVER_DIR) && go test -v ./...

	@echo "🧪 Running tests for GitDB JDBC driver..."
	cd $(JDBC_DIR) && mvn test

clean:
	@echo "🧹 Cleaning JDBC driver build artifacts..."
	cd $(JDBC_DIR) && mvn clean

	@echo "🧹 Cleaning GitDB server build artifacts..."
	cd $(SERVER_DIR) && go clean
